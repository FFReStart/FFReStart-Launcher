# Contributing

## Branches and pull requests

Every independent feature or change gets its own branch named with a type and a kebab-case name (see [Branch naming](#branch-naming)), for example, `feature_control-api-foundation`.

Topic branches start from `development` and are opened as pull requests into `development`. Hotfix branches start from `hotfix` and are opened as pull requests into `hotfix`. A pull request is merged only after approval from another team member. Never push directly to `main`, `development`, or `hotfix`, and never merge your own pull request without approval.

The `development` and `hotfix` branches are merged into `main` only with team approval. Keep each pull request focused on one feature. These rules apply to everyone, including AI coding agents working on a contributor's behalf.

Here you can find the branch strategy used in this project.

<details>
    <summary>
        <strong>Table of contents</strong>
        (click to open)
    </summary>

- [Branch tree structure](#branch-tree-structure)
- [Branch naming](#branch-naming)
</details>

## Branch tree structure

The branch tree structure is a way to organize branches
in a way that makes it easy to find branches and
understand their purpose.

The branch tree structure is as follows:

```bash
main # Production-ready branch
├── hotfix # Where hotfix branches are merged
│   ├── hotfix_hotfix-name # Hotfix branches
├── development # Where feature, fix, refactor, etc, branches are merged
│   ├── chore_chore-name # Chore branches
│   ├── docs_docs-name # Documentation branches
│   ├── feature_feature-name # Feature branches
│   ├── fix_fix-name # Fix branches
│   ├── refactor_refactor-name # Refactor branches
│   ├── security_security-name # Security branches
│   ├── style_style-name # Style branches
│   ├── test_test-name # Test branches
```

- `main` is the production-ready branch.
  It is the branch that is deployed to production.
- `hotfix` is the branch where hotfix branches are merged.
  It is merged into `main` when merged hotfixes are ready to be deployed.
- `hotfix_hotfix-name` is a hotfix branch.
  It is merged into `hotfix` when the hotfix is ready to be deployed.

    This type of branch is used to fix critical bugs that are present in production.

    Only `HOTFIX` commits are allowed in this type of branch.
- `development` is the branch where feature branches are merged.
  It is merged into `main` when the merged branches are ready to be deployed.
- `chore_chore-name` is a chore branch.
  It is merged into `development` when a chore-related commit is ready to be deployed.

    Only `CHORE` commits are allowed in this type of branch.
- `docs_docs-name` is a documentation branch.
  It is merged into `development` when the documentation is ready.

    In source-code repositories, only `DOCS` commits are allowed. In the documentation repository (`ReStart-Backend-Documentation`), only the wiki commit types (`CREATE`, `DELETE`, `FIX`, and `UPDATE`) are allowed.
- `feature_feature-name` is a feature branch.
  It is merged into `development` when the feature is ready to be deployed.

    Only `ENHANCEMENT` and `FEATURE` commits are allowed in this type of branch.
- `fix_fix-name` is a fix branch.
  It is merged into `development` when the fix is ready to be deployed.

    Only `FIX` commits are allowed in this type of branch.
- `refactor_refactor-name` is a refactor branch.
  It is merged into `development` when the refactor is ready to be merged.

    Only `REFACTOR` commits are allowed in this type of branch.
- `security_security-name` is a security branch.
  It is merged into `development` when the security patch is ready to be deployed.

    Only `SECURITY` commits are allowed in this type of branch.
- `style_style-name` is a style branch.
  It is merged into `development` when the style fix is ready to be deployed.

    Only `STYLE` commits are allowed in this type of branch.
- `test_test-name` is a test branch.
  It is merged into `development` when the test is ready to be merged.

    Only `TEST` commits are allowed in this type of branch.

## Branch naming

- All branch names must be in [kebab case](https://www.freecodecamp.org/news/snake-case-vs-camel-case-vs-pascal-case-vs-kebab-case-whats-the-difference/#kebab-case).


Here you will find the commit message standards
that must be followed in order to contribute to this project.

<details>
    <summary>
        <strong>Table of contents</strong>
        (click to open)
    </summary>

- [Main repository (source code repository)](#main-repository-source-code-repository)
- [Wiki repository](#wiki-repository)
</details>

## Main repository (source code repository)

1. First line must have the following structure:

    ```
    <type> - <commit description>
    ```

    - `<type>`:

        Must be any of the following:

        - **`CHORE`:**

            Used to clarify that some chore-related
            changes were applied to the project.

            **E.g:** `CHORE - Update dependencies`

        - **`DOCS`:**

            Used to clarify that some documentation
            of the project was added or updated.

            This may include (but not limited to) updates to `README.md` file.

        - **`ENHANCEMENT`:**

            Used to clarify that some improvements
            were applied to existent functionality.

            **E.g:** `ENHANCEMENT - Add user verification through email`

        - **`FEATURE`:**

            Used to clarify that new specific functionality was added.

            **E.g:** `FEATURE - Add view to register`

        - **`FIX`:**

            Used to clarify that an introduced bug
            in any of functionality or enhancement was fixed.

            **E.g:** `FIX - Animations of Chloe for the character design step`

        - **`GITIGNORE`:**

            Used to clarify that some changes were applied
            to the `.gitignore` file.

            **E.g:** `GITIGNORE - Add .env file`

        - **`HOTFIX`:**

            Used to clarify that an introduced critical bug
            present in production, in any of functionality
            or enhancement was fixed.

            **E.g:** `HOTFIX - Fix bug that prevented users from logging in`

        - **`MERGE`:**

            Used to clarify that a merge of external
            or internal branches was applied.

            **E.g:** `MERGE - Pull request #6`

        - **`REFACTOR`:**

            Used to clarify that some changes were applied
            to the project's codebase in order to improve
            its readability, maintainability, etc.

            **E.g:** `REFACTOR - Improve code readability`

        - **`SECURITY`:**

            Used to clarify that the committed changes are security-related.

            This may include patches to security issues
            or updates of vulnerable dependencies.

        - **`STYLE`:**

            Used to clarify that some changes were applied
            to the project's codebase in order to improve
            its style.

            **E.g:** `STYLE - Add missing semicolons`

        - **`TEST`:**

            Used to clarify that some tests were added,
            updated or removed.

            **E.g:** `TEST - Add unit tests for the user model`

    - `<commit description>`:

        A capitalized, short (50 characters or fewer) summary
        of the committed changes. Cannot include verbs in past tense.

## Wiki repository

The documentation repository (`ReStart-Backend-Documentation`) follows the Wiki repository standards.

1. First line must have the following structure:

    ```
    <type> - <commit description>
    ```

    - `<type>`:

        Must be any of the following:

        - **`CREATE`:**

            Used to clarify that a new page was added to the wiki.

            **E.g:** `CREATE - Add TypeScript Code Standards page`

        - **`DELETE`:**

            Used to clarify that an existent page was deleted from the wiki.

            **E.g:** `DELETE - Remove TypeScript Code Standards page`

        - **`FIX`:**

            Used to clarify that an introduced bug
            in any of wiki pages was fixed.

            **E.g:** `FIX - Fix typo in TypeScript Code Standards page`

        - **`UPDATE`:**

            Used to clarify that an existent page was updated.

            **E.g:** `UPDATE - Add rules for declaring types for objects with unknown properties`

    - `<commit description>`:

        A capitalized, short (50 characters or fewer) summary
        of the committed changes. Cannot include verbs in past tense.
