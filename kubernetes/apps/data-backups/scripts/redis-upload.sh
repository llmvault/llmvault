#!/bin/sh
set -eu

backup_root="${REDIS_BACKUP_ROOT:-/backup}"
backup_time="$(cat "${backup_root}/latest-backup-time")"
backup_dir="${backup_root}/${backup_time}"

case "${REDIS_BACKUP_MODE}" in
  cluster)
    expected_files='leader-0.rdb leader-1.rdb leader-2.rdb cluster-nodes.txt cluster-info.txt manifest.sha256'
    ;;
  standalone)
    expected_files='standalone.rdb manifest.sha256'
    ;;
  *)
    printf 'Unsupported Redis backup mode: %s.\n' "${REDIS_BACKUP_MODE}" >&2
    exit 1
    ;;
esac

aws --endpoint-url "${S3_ENDPOINT}" s3 cp \
  "${backup_dir}/" \
  "s3://${S3_BUCKET}/${S3_PREFIX}/${backup_time}/" \
  --recursive --only-show-errors

for filename in ${expected_files}; do
  local_size="$(wc -c <"${backup_dir}/${filename}" | tr -d ' ')"
  remote_size="$(
    aws --endpoint-url "${S3_ENDPOINT}" s3api head-object \
      --bucket "${S3_BUCKET}" \
      --key "${S3_PREFIX}/${backup_time}/${filename}" \
      --query ContentLength \
      --output text
  )"
  if [ "${remote_size}" != "${local_size}" ]; then
    printf 'Redis backup object size mismatch for %s: local=%s remote=%s.\n' \
      "${filename}" "${local_size}" "${remote_size}" >&2
    exit 1
  fi
done

printf 'Uploaded and verified Redis backup %s.\n' "${backup_time}"
