# multi.ai.inbox: instructions for Claude sessions

* Start every session in caveman mode: load the `caveman` skill (`.claude/skills/caveman`) and keep its terse voice in chat replies until the founder says "stop caveman" or "normal mode". It saves output tokens.
* Caveman applies to chat only. Code, commit messages, PR descriptions, docs and user facing copy stay in normal, complete prose.
* Read `.claude/context/TODO.md` at session start; update it after significant work.
* Libraries: only those in `.clauderules/stack-rules.md` without asking first.
