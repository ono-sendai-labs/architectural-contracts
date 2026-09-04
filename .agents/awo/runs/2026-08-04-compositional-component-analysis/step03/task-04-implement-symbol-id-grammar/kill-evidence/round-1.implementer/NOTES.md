# Lost turn — awo-impl-2026-08-04-compositional-component-analysis-step03-task04 (opencode)

Captured: 2026-09-03T23:11:14Z

Round-1 rework turn. Harness killed the backgrounded wrapper AND an unrelated companion 'sleep 900' in the same instant, 66s after launch. wrapper-signals.log: SIGTERM after 66s; turn pid 113355 GONE; parent 113336 ALIVE. The turn was detached under setsid in its own process session, so a process-group kill could not have reached it -- yet it died. acpx-progress.sh correctly returned verdict 4 DEAD immediately.

## Verdict evidence

- turn pid `113355`: gone
- stopReason in stream: NO
- adapter: pid: 1563 status: running 
- lastPrompt: 2026-09-03T23:09:27.128Z lastExitAt: - disconnectReason: - 
  (lastExitAt AFTER lastPrompt, with no live pid, is a kill. The same
   fields in the other order appear on a healthy just-configured session.)
- the wrapper caught a signal:
      2026-09-03T23:10:33Z SIGTERM after 66s; turn pid 113355 gone; parent 113336 alive
- repository: 6 line(s) of jj diff --stat
