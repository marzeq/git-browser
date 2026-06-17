# git-browser

Minimal web UI for browsing local Git repositories.

`git-browser` scans a directory for Git repositories directly under the configured root or one level below it in organization/user folders, and serves a small read-only interface for:

- listing repositories
- browsing trees and files at any branch, tag, or commit
- viewing repository and file commit history
- listing branches
- showing the top-level `README` file inline
- copying configured SSH and/or HTTPS clone URLs from the page header

Clone URLs are optional. Configure them with:

- `-clone-ssh-prefix=git@example.com:repos/`
- `-clone-https-prefix=https://example.com/repos/`

The repository name is appended directly to each configured prefix.
