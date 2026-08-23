---
description: Close today's learning session and record it in the journal
allowed-tools: Read, Write, Edit, Bash(git log:*), Bash(git diff:*)
---

Close today's session.

1. Look at `git log` and `git diff` for this session. Summarise what changed.
2. Draw a hard line between what Al wrote and what you wrote.
3. Ask one question about the code YOU wrote today. If Al cannot answer it, that
   is an understanding debt and it has to be recorded.
4. Append an entry to `docs/internal/journal.md`, creating the file if it does
   not exist yet, using the format below.

Do not compliment. Record it as it is.

## The entry format

The format lives here rather than in the journal, because a command that says
"use the format already in that file" breaks the moment the file is deleted. That
already happened once: the journal used to live at `docs/learning/journal.md`, the
directory was removed, and this command spent a day pointing at nothing.

`docs/internal/` and not `docs/learning/`, because a learning journal is not part
of the paid deliverable and DEC-004 requires `docs/internal/` to come out of the
repository before it is made public. If the journal should live somewhere else, or
not in the repository at all, change this line.

```markdown
## YYYY-MM-DD

**What changed.** Two or three sentences. Name the files.

**Al wrote.** The specific functions or documents, not "worked on the fixture".

**Claude wrote.** Same standard. If Claude wrote nothing, say so.

**One question Claude asked.** Quote it.

**Understanding debt.** The honest answer to that question, or the admission that
there is not one yet. An unanswered question recorded is worth more than an
answered one invented.
```
