---
id: av-y98v
status: closed
deps: []
links: [av-1rvm, av-p4hm]
created: 2026-07-28T01:22:29Z
type: task
priority: 2
assignee: Max Omdal
---
# display localstorage data in the edit page

to give users maximal control of the data stored for their artifact, give users a dropdown section in an artifact's edit page that will allow them to download an archive of the localstorage data used by the artifact.


## Notes

**2026-08-03T03:54:13Z**

Closed as superseded, not built as written.

Split across two successors:
- DISPLAY half ('display localstorage data in the edit page') shipped as av-hg5f — the edit page's state inspector, with typed per-key form controls, add/delete, and erase-all. Merged on feature/av-p4hm/epic (PR #84).
- DOWNLOAD half ('download an archive of the localstorage data') is NOT built. Carried forward into av-1rvm (draft), where it sits alongside the undo/snapshot question the av-p4hm epic opened up — that epic made state destruction actually work (clear(), erase-all, agent delete_state), so export and recovery are now the same conversation.

Nothing in this ticket is lost; the export requirement is restated in av-1rvm with the open questions it originally lacked (snapshot shape, retention, format as a compatibility surface, whether restore is in scope).
