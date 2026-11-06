# Agent instructions

## Name no external project

This repository is public. Do not name another project in a code
comment, a doc, a test, a commit message or code. A private project
that uses this repository is the main case: do not name it, its paths,
its files, its commits, its benchmarks or its numbers.

- Say the use case in general terms, as "a host that parses many short
  texts", not as the project that does it.
- Measure on inputs in this repository. Do not copy in a result that
  came from another project.
- A dependency of this repository, or the upstream project it ports,
  can be named where the code depends on it.

Check before a commit. Add the names of the private projects you know
to the pattern:

```sh
git grep -n -i -E '<private-project-names>'   # expect nothing
```
