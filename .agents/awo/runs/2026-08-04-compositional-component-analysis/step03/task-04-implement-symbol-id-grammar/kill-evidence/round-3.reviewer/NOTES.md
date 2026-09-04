# Lost turn — awo-rev-2026-08-04-compositional-component-analysis-step03-task04 (codex)

Captured: 2026-09-04T00:14:22Z

SECOND kill of RUN 4, and the first CODEX kill of this run (a reviewer turn). Different ordering from kill #1: wrapper-signals.log records the turn ALIVE at the moment the wrapper caught SIGTERM at 24s, and the turn was gone seconds later when acpx-progress.sh ran. Kill #1 had the turn already dead when the handler ran. Adapter (codex queue owner 68199) survived both.

## Verdict evidence

- turn pid `260471`: gone
- stopReason in stream: NO
- adapter: pid: 68199 status: running 
- lastPrompt: 2026-09-04T00:13:19.434Z lastExitAt: - disconnectReason: - 
  (lastExitAt AFTER lastPrompt, with no live pid, is a kill. The same
   fields in the other order appear on a healthy just-configured session.)
- the wrapper caught a signal:
      2026-09-04T00:13:43Z SIGTERM after 24s; turn pid 260471 alive; parent 260452 alive
- repository: 1 line(s) of jj diff --stat
