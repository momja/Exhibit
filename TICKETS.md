# Ticketing Procedure for Agents
## Adding new tickets
- If you are asked to add a ticket, commit it directly to main. It _must_ be committed to main, to ensure it is not lost.

## Working on a ticket
1. Before creating a feature/bug/chore branch, update the ticket status to "In Progress" in the ticketing system on the main branch.
2. Commit the ticket update to main branch. This is the _only_ commit that should be made to the main branch and it ensures other agents don't pick up the work.
3. Create a new branch for the feature/bug/chore and continue development.
4. A ticket should be 'closed' on the feature branch before a PR is created, otherwise merging the PR will not have a correct state on `main`
