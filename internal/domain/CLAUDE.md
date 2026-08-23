# Zones inside internal/domain

This directory holds two things with different owners, and the split is by file.

| File | Zone | Owner |
| --- | --- | --- |
| `compute.go` | RED | Al alone. Claude has no write permission |
| `types.go` | YELLOW | Claude may write, and must explain each design decision in three sentences and name one rejected alternative |
| `arch_test.go`, `*_test.go` | YELLOW | same, except tests for `compute.go` which Al asks for explicitly |

## Why the zone is a file and not a directory

It used to be a directory, `internal/depth`. Methodology 1.0.3 moved the
computations into this package and left the lock pointing at a directory that no
longer held anything, so for a while the red zone existed in `CLAUDE.md` and
nowhere else. The zone follows the code, not the name. The empty directory was
removed on 23 August 2026 and this file is the only zone document for the
methodology code now.

`compute.go` is the core of Keel's methodology and it is the paid deliverable. Al
has to be able to defend every number in it to a reviewer or a funder. If Claude
writes it, Al cannot. A type is a shape and a formula is a claim, and only the
second one has to be defended, which is why `types.go` next door stays open.

Enforced rather than agreed: `Edit` and `Write` are denied on `compute.go` in
`.claude/settings.json`, and the Bash path is closed by
`.claude/hooks/lindungi-zona-merah.sh`.

**Three routes, all closed, and the third one caught Claude out.** A Bash command
reaches this file by naming it, by naming this directory, or by sweeping a tree that
contains it. Only the first was closed until 24 August 2026, and the last is the one
worth knowing about in daily work:

| Want to | Do this | Not this |
| --- | --- | --- |
| format | `gofmt -w internal/domain/types.go` | `gofmt -w internal/domain/` |
| check formatting | `gofmt -l .`, which is read only | `make fmt`, which writes |

`make fmt` is refused for Claude and unchanged for Al, whose terminal this hook does
not sit in. So CI's gofmt check still has an owner and a one command fix, and Claude
formats by naming files. Finding P2-6c.

## Claude's role in compute.go

Allowed: read the code, run the tests, point out an edge case that is not handled,
correct a misunderstanding about Go, and write tests in `*_test.go` if Al asks for
them explicitly.

Not allowed: writing or editing the implementation, pasting a complete block for Al
to copy, or an "example" that happens to be the answer.

## If Al asks Claude to write it

Refuse, then ask one question that points at the thing being decided. For example:

> If the best SDEX ask sits at 0.101 and the AMM curve only crosses 0.101 after 400
> units, how many units can be absorbed before the combined marginal price moves?
> Which one runs out first?

## Conceptual traps to watch for

- Summing SDEX and AMM depth separately. This is WRONG. Both compete at the same
  marginal price. See `docs/methodology/04-depth.md` section 3.
- Using `float64` anywhere, including as an intermediate value.
- Assuming the order book always has both sides.
- Forgetting that an AMM is a continuous curve, not a set of discrete levels.
- Reading `Cost` without reading `Reachable`. The two are computed from disjoint
  sets of asks and a cost to an unreachable target is not a cost.

## The cleanup Claude could not do, done

Al ran `git rm -r internal/depth` on 23 August 2026. The directory held nothing
but its own `CLAUDE.md` in Indonesian, and Claude could not remove it, because the
lock that makes a zone real also makes it unremovable from this side.

Three things went with it, and each one had to be retired by hand rather than
disappearing along with the directory:

- The `internal/depth/**` denials in `.claude/settings.json` and the path in
  `.claude/hooks/lindungi-zona-merah.sh`. A lock on a path that does not exist
  cannot be distinguished from a lock that is working.
- The `../depth` entry in `paketMurni` in `arch_test.go`, and the
  `os.IsNotExist` tolerance that existed only to let that entry point at nothing.
- The single untranslatable exception DEC-005 recorded, which was that Indonesian
  file. DEC-005 section 2 records it as retired by deletion rather than by
  translation.

That list is the point. A directory is deleted in one command and the things that
referred to it are not, and every one of them fails silently rather than loudly:
a deny rule matching no path, a linter exclusion matching no file, and a purity
scan over a directory that is not there all report success.
