#!/usr/bin/env sh

REMOTE_URL=
REMOTE_BRANCH=

UPSTREAM_URL=
UPSTREAM_BRANCH=
UPSTREAM_NAME="upstream"

CLONE_DIR="/opt/downstream_repo"


sync_upstream() {
  git clone "${REMOTE_URL}" "${CLONE_DIR}"
  CURRENT_DIR=$(pwd)
  cd "${CLONE_DIR}" || exit 1
  git remote add "${UPSTREAM_NAME}" "${UPSTREAM_URL}"
  git fetch "${UPSTREAM_NAME}" "${UPSTREAM_URL}"

  LAST_UPSTREAM_COMMIT_SHA=$(git rev-parse --short "${UPSTREAM_NAME}/${UPSTREAM_BRANCH}")
  LAST_UPSTREAM_COMMIT_MSG=$(git log -1 --pretty=%B "${UPSTREAM_NAME}/${UPSTREAM_BRANCH}")
  SYNC_COMMIT_MSG="[sync] ${LAST_UPSTREAM_COMMIT_SHA} - ${LAST_UPSTREAM_COMMIT_MSG}"
  CURRENT_BRANCH=$(git branch --show-current)

  if [ "${REMOTE_BRANCH}" != "${CURRENT_BRANCH}" ]; then
    git checkout "${REMOTE_BRANCH}"
  fi

  LAST_UPSTREAM_SHA_CONTAINING_BRANCH_NAME="$(git branch --contains "${LAST_UPSTREAM_COMMIT_SHA}" 2>/dev/null | cut -d " " -f2)"
  if [ "${LAST_UPSTREAM_SHA_CONTAINING_BRANCH_NAME}" != "${REMOTE_BRANCH}" ]; then
    printf "we need to sync,\n%s is not in %s\n" \
      "${LAST_UPSTREAM_COMMIT_SHA}" \
      "${REMOTE_BRANCH}"
    git merge "${UPSTREAM_NAME}/${UPSTREAM_BRANCH}" -m "${SYNC_COMMIT_MSG}" --ff
  git add .
  git commit -m "${SYNC_COMMIT_MSG}" || true
  git push
  else
    echo "we are already in sync"
    echo "${LAST_UPSTREAM_COMMIT_SHA} is already merged into ${REMOTE_BRANCH}"
  fi

  printf "removing temp repo folder %s\n" "${CLONE_DIR}"
  rm -rf "${CLONE_DIR}" || return 1
  cd "${CURRENT_DIR}" || return 1
}

validate_args() {
  if [ -z "${REMOTE_URL}" ] || [ -z "${REMOTE_BRANCH}" ] || [ -z "${UPSTREAM_URL}" ] || [ -z "${UPSTREAM_BRANCH}" ]; then
    printf "missing required argument\n" >&2
    return 1
  fi
}

print_vars() {
  printf "REMOTE_URL=%s\n" "${REMOTE_URL}"
  printf "REMOTE_BRANCH=%s\n" "${REMOTE_BRANCH}"
  printf "UPSTREAM_URL=%s\n" "${UPSTREAM_URL}"
  printf "UPSTREAM_BRANCH=%s\n" "${UPSTREAM_BRANCH}"
  printf "UPSTREAM_NAME=%s\n" "${UPSTREAM_NAME}"
}

usage() {
  printf "%s remote_url remote_branch upstream_url upstream_branch [upstream_name]" "$(basename "$0")"
}

#######################

REMOTE_URL="$1"
REMOTE_BRANCH="$2"
UPSTREAM_URL="$3"
UPSTREAM_BRANCH="$4"

if [ -n "$5" ]; then
  UPSTREAM_NAME="$5"
fi

if ! validate_args; then
  usage
  exit 1
fi

print_vars || exit 1
sync_upstream || exit 1
