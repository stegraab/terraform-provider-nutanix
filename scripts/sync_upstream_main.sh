#!/usr/bin/env bash

set -euo pipefail

UPSTREAM_REMOTE="${UPSTREAM_REMOTE:-upstream}"
FORK_REMOTE="${FORK_REMOTE:-origin}"
UPSTREAM_BRANCH="${UPSTREAM_BRANCH:-master}"
MIRROR_BRANCH="${MIRROR_BRANCH:-upstream-main}"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Working tree is not clean. Commit/stash changes first."
  exit 1
fi

echo "Fetching latest from ${UPSTREAM_REMOTE}..."
git fetch "${UPSTREAM_REMOTE}" --prune --tags

echo "Resetting ${MIRROR_BRANCH} to ${UPSTREAM_REMOTE}/${UPSTREAM_BRANCH}..."
git checkout -B "${MIRROR_BRANCH}" "${UPSTREAM_REMOTE}/${UPSTREAM_BRANCH}"

echo "Pushing ${MIRROR_BRANCH} to ${FORK_REMOTE}..."
git push --force-with-lease "${FORK_REMOTE}" "${MIRROR_BRANCH}"

echo "Syncing tags from ${UPSTREAM_REMOTE} to ${FORK_REMOTE}..."
while IFS= read -r tag_ref; do
  tag="${tag_ref#refs/tags/}"
  git push "${FORK_REMOTE}" "refs/tags/${tag}:refs/tags/${tag}"
done < <(git ls-remote --tags --refs "${UPSTREAM_REMOTE}" | awk '{print $2}')

echo "Done. ${MIRROR_BRANCH} now mirrors ${UPSTREAM_REMOTE}/${UPSTREAM_BRANCH}, and upstream tags are present on ${FORK_REMOTE}."
