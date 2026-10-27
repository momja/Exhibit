---
id: Exh-g0fx
status: closed
deps: []
links: []
created: 2026-10-27T20:09:04Z
type: bug
priority: 1
assignee: Max Omdal
tags: [perf, mobile, auth]
---
# Gallery pages download ClerkJS and Clerk UI eagerly for the user button

In hosted mode every gallery page loads @clerk/ui and @clerk/clerk-js (several hundred KB of JS from the Clerk Frontend API) and calls clerk.user.reload(), only to mount the user button. On a phone that is network plus parse/compile cost on every launch. Fix: defer loading Clerk until the account control is used, and measure before and after under mobile throttling.

