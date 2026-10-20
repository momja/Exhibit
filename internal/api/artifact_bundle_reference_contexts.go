package api

import (
	"errors"
	"mime"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"github.com/tdewolff/parse/v2/js"
)

type artifactBundleReferenceLanguage uint8

const (
	artifactBundleText artifactBundleReferenceLanguage = iota
	artifactBundleHTML
	artifactBundleCSS
	artifactBundleJavaScript
)

func artifactBundleLanguage(contentType string) artifactBundleReferenceLanguage {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch mediaType {
	case "text/html", "application/xhtml+xml", "image/svg+xml":
		return artifactBundleHTML
	case "text/css":
		return artifactBundleCSS
	// JavaScript MIME essences used by HTML, including the legacy aliases:
	// https://mimesniff.spec.whatwg.org/#javascript-mime-type
	case "application/ecmascript", "application/javascript", "application/x-ecmascript", "application/x-javascript",
		"text/ecmascript", "text/javascript", "text/javascript1.0", "text/javascript1.1", "text/javascript1.2",
		"text/javascript1.3", "text/javascript1.4", "text/javascript1.5", "text/jscript", "text/livescript",
		"text/x-ecmascript", "text/x-javascript", "application/json", "text/json":
		return artifactBundleJavaScript
	}
	if strings.HasSuffix(mediaType, "+json") {
		return artifactBundleJavaScript
	}
	return artifactBundleText
}

type artifactBundleReferenceSpan struct {
	text             string
	offset           int
	quote            byte
	encodedString    bool
	srcset           bool
	literalAttribute bool
	sourceOffset     func(int) int
	depth            int
	budget           *int64
	err              error
}

type artifactBundleReferenceSpans func() (artifactBundleReferenceSpan, bool)

func artifactBundleSpans(content string, language artifactBundleReferenceLanguage) artifactBundleReferenceSpans {
	remaining := 8*int64(len(content)) + 8192
	span := artifactBundleReferenceSpan{text: content, budget: &remaining}
	return artifactBundleCodeSpans(span, language)
}

func artifactBundleCodeSpans(source artifactBundleReferenceSpan, language artifactBundleReferenceLanguage) artifactBundleReferenceSpans {
	if source.depth >= 32 || source.budget != nil && int64(len(source.text)) > *source.budget {
		return func() (artifactBundleReferenceSpan, bool) {
			return artifactBundleReferenceSpan{err: errors.New("artifact bundle reference parsing exceeds its nesting or work limit")}, true
		}
	}
	if source.budget != nil {
		*source.budget -= int64(len(source.text))
	}
	if language == artifactBundleHTML {
		nested := artifactBundleHTMLSpans(source.text, source.depth+1, source.budget)
		return func() (artifactBundleReferenceSpan, bool) {
			span, ok := nested()
			if !ok || span.err != nil {
				return span, ok
			}
			if span.sourceOffset == nil && source.sourceOffset == nil {
				span.offset += source.offset
			} else {
				innerOffset := span.sourceOffset
				span.sourceOffset = func(at int) int {
					if innerOffset != nil {
						at = innerOffset(at)
					}
					at += source.offset
					if source.sourceOffset != nil {
						at = source.sourceOffset(at)
					}
					return at
				}
			}
			return span, true
		}
	}
	if language != artifactBundleCSS && language != artifactBundleJavaScript {
		done := false
		return func() (artifactBundleReferenceSpan, bool) {
			if done {
				return artifactBundleReferenceSpan{}, false
			}
			done = true
			return source, true
		}
	}
	source.literalAttribute = false // code tokens supply their own string delimiters
	// Input owns one copy of this context's bytes. Tokens refer to that buffer;
	// comment count does not affect retained memory.
	buffer := make([]byte, len(source.text)+1) // reserve the lexer's sentinel byte
	copy(buffer, source.text)
	input := parse.NewInputBytes(buffer[:len(source.text)])
	if language == artifactBundleCSS {
		return artifactBundleCSSSpans(source, input)
	}
	// The lexer handles ordinary comments, but not the initial hashbang. Skip
	// it in the input rather than slicing/replacing source bytes, so every token
	// still has its original byte offset. A decoded script may start with a BOM.
	start := 0
	if strings.HasPrefix(source.text, "\uFEFF") {
		start = len("\uFEFF")
	}
	if strings.HasPrefix(source.text[start:], "#!") {
		if end := strings.IndexAny(source.text[start:], "\r\n\u2028\u2029"); end >= 0 {
			start += end
		} else {
			start = len(source.text)
		}
	}
	input.Move(start)
	input.Skip()
	lexer := js.NewLexer(input)
	goal := artifactBundleJavaScriptGoal{expression: true, statement: true}
	return func() (artifactBundleReferenceSpan, bool) {
		for {
			token, data := lexer.Next()
			if (token == js.DivToken || token == js.DivEqToken) && goal.expression {
				token, data = lexer.RegExp()
			}
			if token == js.ErrorToken {
				return artifactBundleReferenceSpan{}, false
			}
			goal.advance(token)
			start, end := input.Offset()-len(data), input.Offset()
			span := source
			span.encodedString = true
			switch token {
			case js.StringToken, js.TemplateToken, js.TemplateEndToken:
				span.quote = source.text[start]
				if token == js.TemplateEndToken {
					span.quote = '`'
				}
				start++
				end--
			case js.TemplateStartToken, js.TemplateMiddleToken:
				span.quote = '`'
				start++
				end -= 2 // ${ belongs to the next expression, not its string.
			default:
				continue
			}
			span.text, span.offset = source.text[start:end], source.offset+start
			return span, true
		}
	}
}

// Yield ordinary CSS in runs as well as string/url tokens. This keeps bare
// historical paths discoverable without ever splitting a URL at comment-like
// bytes inside url(), or allowing a BadURL token to expose a nested path.
func artifactBundleCSSSpans(source artifactBundleReferenceSpan, input *parse.Input) artifactBundleReferenceSpans {
	lexer := css.NewLexer(input)
	var token css.TokenType
	var start, end int
	pending := false
	done := false
	return func() (artifactBundleReferenceSpan, bool) {
		for !done {
			if pending {
				pending = false
				if token == css.ErrorToken {
					done = true
					break
				}
				if token != css.URLToken && token != css.StringToken {
					continue
				}
				if token == css.URLToken {
					start += strings.IndexByte(source.text[start:end], '(') + 1
					if end > start && source.text[end-1] == ')' {
						end--
					}
				}
				for start < end && artifactBundleSpace(source.text[start]) {
					start++
				}
				for start < end && artifactBundleSpace(source.text[end-1]) {
					end--
				}
				span := source
				span.quote = 0 // HTML's enclosing quote belongs to the attribute.
				if start < end && (source.text[start] == '\'' || source.text[start] == '"') {
					span.quote = source.text[start]
					start++
					if end > start && source.text[end-1] == span.quote {
						end--
					}
				}
				span.text, span.offset = source.text[start:end], source.offset+start
				return span, true
			}
			runStart := input.Offset()
			for !pending {
				var data []byte
				token, data = lexer.Next()
				end = input.Offset()
				start = end - len(data)
				switch token {
				case css.ErrorToken, css.CommentToken, css.URLToken, css.StringToken, css.BadURLToken, css.BadStringToken:
					pending = true
				}
			}
			if start > runStart {
				span := source
				span.text, span.offset, span.quote = source.text[runStart:start], source.offset+runStart, 0
				return span, true
			}
		}
		return artifactBundleReferenceSpan{}, false
	}
}

// RegExp is a separate ECMAScript lexical goal. Track statement lists, member
// names and expression delimiters so a regex after a block is distinct from
// division after an object/function expression. Only nesting consumes memory;
// the lexer handles escapes, character classes, comments and template contents.
type artifactBundleJavaScriptGoal struct {
	expression       bool
	statement        bool
	previous         js.TokenType
	delimiters       []artifactBundleJavaScriptDelimiter
	asyncDeclaration bool
	label            bool
	control          js.TokenType
	switchBlock      bool
	memberLineBreak  bool
}

type artifactBundleJavaScriptDelimiter struct {
	kind       js.TokenType
	expression bool // whether closing this context permits a regex
	block      bool // contents are a statement list, rather than object members
	classBody  bool
	memberBody bool // class method/static block, rather than a field initializer
	switchBody bool
	caseClause bool
	member     bool // next name is an object/class member, including reserved words
}

func (goal *artifactBundleJavaScriptGoal) context() *artifactBundleJavaScriptDelimiter {
	if len(goal.delimiters) == 0 {
		return nil
	}
	return &goal.delimiters[len(goal.delimiters)-1]
}

func (goal *artifactBundleJavaScriptGoal) advance(token js.TokenType) {
	if token == js.LineTerminatorToken || token == js.CommentLineTerminatorToken {
		// Restricted productions end before a newline. In particular, the brace
		// after return\n starts a block, while return { ... } returns an object.
		if goal.previous == js.ReturnToken || goal.previous == js.BreakToken || goal.previous == js.ContinueToken {
			goal.expression, goal.statement = true, true
		}
		if parent := goal.context(); parent != nil && parent.classBody && !parent.member && !goal.expression &&
			goal.previous != js.DotToken && goal.previous != js.OptChainToken {
			goal.memberLineBreak = true
		}
		return
	}
	if token == js.WhitespaceToken || token == js.CommentToken {
		return
	}
	if goal.memberLineBreak {
		// A name following a completed field expression starts a new class
		// member after ASI. Operators and calls can continue the initializer.
		if parent := goal.context(); parent != nil && parent.classBody &&
			(js.IsIdentifierName(token) && token != js.InToken && token != js.InstanceofToken ||
				token == js.PrivateIdentifierToken || token == js.StringToken || js.IsNumeric(token)) {
			parent.member = true
		}
		goal.memberLineBreak = false
	}
	label, control, switchBlock := goal.label, goal.control, goal.switchBlock
	goal.label, goal.control, goal.switchBlock = false, 0, false
	if (goal.previous == js.DotToken || goal.previous == js.OptChainToken) &&
		(js.IsIdentifierName(token) || token == js.PrivateIdentifierToken) {
		// Normalize the name too: obj.if() is a call, never a control header.
		goal.expression, goal.statement, goal.previous = false, false, js.IdentifierToken
		goal.asyncDeclaration = false
		return
	}
	parent := goal.context()
	if parent != nil && parent.member && token != js.CloseBraceToken {
		modifier := token == js.GetToken || token == js.SetToken || token == js.AsyncToken ||
			parent.classBody && token == js.StaticToken
		if js.IsIdentifierName(token) || token == js.PrivateIdentifierToken || token == js.StringToken || js.IsNumeric(token) {
			parent.member = modifier
			goal.expression, goal.statement, goal.previous = false, false, js.IdentifierToken
			if parent.classBody && token == js.StaticToken {
				goal.previous = js.StaticToken // static { ... } contains statements.
			}
			goal.asyncDeclaration = false
			return
		}
		if token == js.MulToken { // generator method prefix
			goal.expression, goal.statement, goal.previous = true, false, token
			return
		}
		parent.member = false
	}
	asyncDeclaration := goal.asyncDeclaration
	goal.asyncDeclaration = token == js.AsyncToken && goal.statement
	goal.label = goal.statement && js.IsIdentifier(token)
	switch token {
	case js.FunctionToken, js.ClassToken:
		declaration := goal.statement || goal.previous == js.ExportToken || goal.previous == js.DefaultToken ||
			goal.previous == js.AsyncToken && asyncDeclaration
		goal.delimiters = append(goal.delimiters, artifactBundleJavaScriptDelimiter{kind: token, expression: declaration})
		goal.expression, goal.statement = true, false
	case js.IfToken, js.WhileToken, js.ForToken, js.WithToken, js.SwitchToken:
		goal.control = token
		goal.expression, goal.statement = true, false
	case js.CatchToken:
		goal.control = token
		goal.expression, goal.statement = true, true // binding parentheses are optional
	case js.AwaitToken:
		if control == js.ForToken {
			goal.control = control // for await (...) also has a statement body.
		}
		goal.expression, goal.statement = true, false
	case js.OpenParenToken:
		goal.delimiters = append(goal.delimiters, artifactBundleJavaScriptDelimiter{
			kind: token, expression: control != 0, switchBody: control == js.SwitchToken,
		})
		goal.expression, goal.statement = true, false
	case js.OpenBraceToken:
		block := goal.statement || goal.previous == js.CloseParenToken || goal.previous == js.ArrowToken
		context := artifactBundleJavaScriptDelimiter{
			kind: token, block: block, expression: block && goal.previous != js.ArrowToken, switchBody: switchBlock,
		}
		if parent != nil && (parent.kind == js.FunctionToken || parent.kind == js.ClassToken) {
			context.classBody = parent.kind == js.ClassToken
			context.block, context.expression = !context.classBody, parent.expression
			goal.delimiters = goal.delimiters[:len(goal.delimiters)-1]
		} else if parent != nil && parent.classBody && goal.previous == js.StaticToken {
			context.block, context.expression, context.memberBody = true, false, true
		} else if parent != nil && parent.classBody && goal.previous == js.CloseParenToken {
			context.memberBody = true
		}
		context.member = !context.block
		goal.delimiters = append(goal.delimiters, context)
		goal.expression, goal.statement = true, context.block
	case js.OpenBracketToken, js.TemplateStartToken, js.QuestionToken:
		goal.delimiters = append(goal.delimiters, artifactBundleJavaScriptDelimiter{kind: token})
		goal.expression, goal.statement = true, false
	case js.CloseParenToken, js.CloseBraceToken, js.CloseBracketToken, js.TemplateEndToken:
		goal.expression, goal.statement = false, false
		if parent != nil && (token == js.CloseParenToken && parent.kind == js.OpenParenToken ||
			token == js.CloseBraceToken && parent.kind == js.OpenBraceToken ||
			token == js.CloseBracketToken && parent.kind == js.OpenBracketToken ||
			token == js.TemplateEndToken && parent.kind == js.TemplateStartToken) {
			goal.expression, goal.statement = parent.expression, parent.expression
			goal.switchBlock = token == js.CloseParenToken && parent.switchBody
			goal.delimiters = goal.delimiters[:len(goal.delimiters)-1]
			if outer := goal.context(); parent.memberBody && outer != nil && outer.classBody {
				outer.member = true
				goal.expression, goal.statement = false, false
			}
		}
	case js.ColonToken:
		goal.expression, goal.statement = true, false
		if parent != nil && parent.kind == js.QuestionToken {
			goal.delimiters = goal.delimiters[:len(goal.delimiters)-1]
		} else if label || parent != nil && parent.switchBody && parent.caseClause {
			goal.statement = true
			if parent != nil {
				parent.caseClause = false
			}
		}
	case js.CaseToken, js.DefaultToken:
		if parent != nil && parent.switchBody {
			parent.caseClause = true
		}
		goal.expression, goal.statement = true, false
	case js.SemicolonToken:
		goal.expression = true
		goal.statement = parent == nil || parent.kind == js.OpenBraceToken && parent.block
		if parent != nil && parent.classBody {
			parent.member = true
		}
	case js.CommaToken:
		if parent != nil && parent.kind == js.OpenBraceToken && !parent.block && !parent.classBody {
			parent.member = true
		}
		goal.expression, goal.statement = true, false
	case js.ElseToken, js.DoToken, js.TryToken, js.FinallyToken:
		goal.expression, goal.statement = true, true
	case js.TemplateMiddleToken:
		goal.expression, goal.statement = true, false
	case js.StringToken, js.TemplateToken, js.RegExpToken,
		js.ThisToken, js.SuperToken, js.NullToken, js.TrueToken, js.FalseToken,
		js.DotToken, js.OptChainToken:
		goal.expression, goal.statement = false, false
	case js.IncrToken, js.DecrToken:
		// Prefix operators still need their operand; postfix operators have
		// already completed it. Keep that distinction across a newline.
		goal.statement = false
	default:
		goal.expression = !js.IsNumeric(token) && !js.IsIdentifier(token) && token != js.PrivateIdentifierToken
		goal.statement = false
	}
	goal.previous = token
}

func artifactBundleSpace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\r' || character == '\n' || character == '\f'
}
