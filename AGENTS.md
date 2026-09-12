# Agent instructions

## MANDATORY: branches and pull requests

Agents never commit to or push `main`, `development`, or `hotfix`. They create a branch named with a type and a kebab-case name from `development` (or a `hotfix_` branch from `hotfix`). They write commit messages as `<TYPE> - <Description>`, capitalised, 50 characters or fewer, with no past tense, using only the commit types `CONTRIBUTING.md` allows for that branch type. They push the branch, open a pull request into `development` (hotfixes into `hotfix`), and stop. A human approves and merges.
