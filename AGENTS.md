
# Project Instructions for AI Agents

This file provides instructions and context for AI coding agents working on this project.

## Coding Best Practices
- Follow John Ousterhout's advice, and pull complexity downwards. Write clean, readable code.
- **Naming Conventions:** Good names have two properties: precision and consistency.
  - The greater the distance between a name’s declaration and its uses, the longer the name should be.

## UI Conventions
- **Icons:** all new UI uses **Phosphor Icons**, self-hosted / embedded on the app origin (never a third-party CDN). See `docs/technical_stack.md` §1 (stack table) and §9 (Gallery UI) for the load method and markup pattern.

## Version Control
- Always keep the main worktree clean and on `main`. The only exception is creation/modification of tickets. Those may be committed directly to main.
- **Never** develop on the `main` branch. Use the standard branch names `feature/{id}/{description}` or `bug/{id}/{description}` where `id` is the ticket ID.
- **Never** merge directly to `main` and **Never** push main. You can push all non-release branches. If working on multiple dependent issues, create a merge branch separate from `main`.

## Ticket System
This project uses a CLI ticket system for task management. Run `tk help` when you need to use it.

Read @TICKETS.md for more details.

## Product

@docs/product_requirement_doc.md

## Architecture Overview

@docs/architecture.md

## Tech Stack

@docs/technical_stack.md

## Documentation

All docs are in `./docs/` directory.

### Screenshots

Agents can take screenshots using the CLI tool `shot-scraper`.
Screenshots should stay up to date with UI changes.
