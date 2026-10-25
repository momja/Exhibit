---
id: Exh-04g8
status: closed
deps: []
links: []
created: 2026-10-25T18:32:22Z
type: bug
priority: 2
assignee: Max Omdal
---
# Restore direct ticket pushes with migration checks enabled

The Migration versions required-check ruleset also blocks direct ticket-only pushes to main. Preserve the established ticket workflow while retaining migration collision checks on PRs. Use a repository SSH deploy key for Git pushes from the T3 checkout and grant deploy keys bypass only on main. The owner's account must have no bypass, so PR merges through GitHub still require the migration check.

## Acceptance Criteria

Git pushes from the T3 checkout use the deploy key; the bypass targets only refs/heads/main; the owner's account has no bypass; the migration CI job and strict base freshness requirement remain active; tracked ruleset configuration matches live settings. Private key material stays outside the repository.


## Notes

**2026-10-25T18:56:12Z**

Configured the T3 Exhibit Git pushes write-enabled deploy key (GitHub key ID 167636243). The shared local repository uses an SSH origin push URL and a repository-specific core.sshCommand selecting that key; HTTPS fetches are unchanged. Applied ruleset 25791766 with only the DeployKey actor and explicit refs/heads/main scope. SSH authentication confirms the repository deploy-key identity; GitHub reports current_user_can_bypass=never for momja. Required Migration versions check, GitHub Actions app binding, and strict base freshness remain intact. No push to main was performed. Deploy-key bypass is for Git pushes and is not restricted to ticket file paths.
