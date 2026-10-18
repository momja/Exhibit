# PR-Agent reviews

Exhibit runs [The-PR-Agent/pr-agent](https://github.com/The-PR-Agent/pr-agent)
through [.github/workflows/pr-agent.yml](../.github/workflows/pr-agent.yml), using
PR-Agent's default OpenAI models.

## Enable the action

1. Add an Actions repository secret named `OPENAI_KEY` in
   [Exhibit's Actions secrets settings](https://github.com/momja/Exhibit/settings/secrets/actions).
   Set its value to an OpenAI API key with API billing and access to PR-Agent's
   default models. Alternatively, run `gh secret set OPENAI_KEY --repo momja/Exhibit`
   and enter the key at the prompt.
2. Merge the workflow and configuration into `main`. The initial setup PR does
   not run this workflow: `pull_request_target` uses the base branch's workflow,
   and `issue_comment` requires the workflow on the default branch.
3. Open a non-draft PR, push a commit to one, or comment `/review` on an existing
   PR. Check the **PR Agent** workflow in GitHub Actions and the bot's review comment.

GitHub supplies `GITHUB_TOKEN`; no personal access token is needed. The job fails
with a setup message if `OPENAI_KEY` is missing. Review calls use the key's OpenAI
API billing and send PR content to OpenAI.

## Automatic reviews and commands

The action reviews non-draft PRs when opened, reopened, marked ready for review,
or updated with new commits. It updates a persistent review comment. Automatic
description generation and code suggestions are disabled.

Repository owners, organization members, and collaborators can post commands at
the start of a new PR conversation comment:

- `/review` — review the current diff again.
- `/ask <question>` — ask about the changes.
- `/improve` — request code suggestions.
- `/describe` — request an AI summary as a comment, preserving the PR description.
- `/help` — list available commands.

Bot events, ordinary issue comments, and comments from other users do not run the
job. Commands use new conversation comments, rather than edits or inline review
comments. New PR updates cancel an older automatic run for that PR; separate
comment commands can each finish.

The workflow uses `pull_request_target` so fork PRs can also be reviewed. It never
checks out or executes PR code; PR-Agent fetches the diff and trusted repository
configuration through GitHub's API. Keep builds and other execution of PR content
out of this job.

## Configuration and upgrades

[.pr_agent.toml](../.pr_agent.toml) contains Exhibit-specific review guidance and
the `/describe` comment setting. PR-Agent also reads `AGENTS.md` from the default
branch. Set model overrides with `CONFIG__MODEL` and `CONFIG__FALLBACK_MODELS` in
the action step's environment if a different OpenAI model is needed; the latter
is a JSON array string. Automatic tool and push-trigger settings live in the
workflow's `GITHUB_ACTION_CONFIG__*` environment variables. In particular,
`HANDLE_PUSH_TRIGGER` and `PUSH_COMMANDS` enable reviews on new commits.

The runtime is pinned to the Docker digest of `pragent/pr-agent:0.48.0-github_action`.
A source action pinned to a Git SHA still loads the upstream rolling
`github_action` image, so this workflow pins the runtime directly. To upgrade,
review the upstream release notes, resolve the new version's digest with
`docker buildx imagetools inspect pragent/pr-agent:<version>-github_action`, then
update the workflow digest and version comment together with this document.

See the upstream [GitHub Action guide](https://docs.pr-agent.ai/installation/github/)
and [Docker pinning guidance](https://github.com/The-PR-Agent/pr-agent/blob/main/SECURITY.md#enhanced-security-with-docker-digest).
