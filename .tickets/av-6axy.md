---
id: av-6axy
status: open
deps: []
links: [av-v991]
created: 2026-09-06T01:33:10Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-7k7b
tags: [security, render, sharing]
---
# Render token: named claims and a viewer principal separate from the owner

Slice 2 of sharing v1. Independent of the schema work (av-lrae), so it can be
built in parallel, but av-awr4 and av-v991's shared board both need it.

Full design is the 2026-09-05 'TOKEN ENCODING, CONCRETE' note on av-v991, which
supersedes the 2026-08-06 21:48 sketch there. Two of that sketch's specifics are
wrong; read the newer note.

## Why now

rendertoken.Claims.OwnerID does two jobs today: it authorizes the read (checked
against the artifact's owner) and it selects whose state gets inlined. Same
person on every route that exists. On a directed share they are different
people, so this is av-q0ub's two-principal split arriving at the token.

Positional encoding (owner.exp[.a].tag) cannot absorb a third optional field
without becoming fragile, which is why the encoding change comes first. Doing it
after the new claim is the same work plus a format migration.

## Claim set

    o   owner whose artifact this is; authorizes the read      required
    e   expiry, unix seconds                                   required
    p   principal: whose state rows to inline                  absent = same as o
    a   anonymous: no principal at all                         absent = not anonymous
    s   the share row this render happens under                absent = not a share

p defaulting to o keeps every token minted today byte-identical.

'a' stays its own key rather than collapsing to p=0. av-wmp6's argument is that
anonymous SUBTRACTS authority and must be unforgeable; p=0 invites a 'zero is
falsy, treat as unset' bug that silently promotes a nobody to the owner. A token
carrying both a and p is rejected at parse, not resolved.

## Wire

    o=1.e=1785948001.<tag>                  today's token plus one byte
    o=1.e=1785948001.p=7.<tag>              grant: viewer 7's rows
    o=1.e=1785948001.a=1.<tag>              anonymous
    o=1.e=1785948001.p=1.s=shr_abc.<tag>    share

No version in the wire (it stays inside the MAC, and the 10-minute TTL buys what
a wire version would). No base64 wrapping (33% on a URL value, and
LastIndexByte('.') already cuts the tag unambiguously). Reject duplicate keys —
the load-bearing rule. Reject unknown keys. No value may contain '.' or '=',
enforced at mint and at parse. Sorted key order makes the encoding canonical.

Keep the properties the package already has: the tag is the last field, the
artifact id is mixed into the MAC rather than carried, so adding a claim changes
what is parsed and never what is authenticated.

## Out of scope

No w=1 write claim. v1's recipient is authenticated and framed, so persistState
reaches the host frame and the host does the authenticated PUT — see av-v991's
'CORRECTION' note. A write claim also belongs nowhere near a URL (av-rgp1).

## Acceptance Criteria

1. A token minted with no p and no a is byte-identical to what this package
   mints today, so nothing in flight breaks on deploy.
2. Verify rejects a duplicate key, an unknown key, and a token carrying both
   a and p.
3. Verify rejects a value containing '.' or '='; mint refuses to produce one.
4. A token minted for artifact A still fails to verify on artifact B — the MAC
   scoping is unchanged.
5. Claims exposes the viewer principal separately from the owner, and a render
   inlines the principal's rows.
6. Claims are parsed only after the MAC checks out.
7. Every existing rendertoken test still passes.

