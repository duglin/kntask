#!/bin/bash

[[ -n "${K_JOB_INDEX}" ]] && index=" | Index: ${K_JOB_INDEX}"
[[ -n "${*}" ]] && args=" | Args: $*"

echo "In app ${index} ${args} $(hostname)"
echo sleeping ${SLEEP:-1}
sleep ${SLEEP:-1}
env | sort | grep -v -e JOBCONTROLLER -e TEST_ -e KUBERNETES -e K_TASK_HEADERS

# Fail 1/3 of the time
if [[ -n "${K_JOB_NAME}" ]] && (( $RANDOM % 3 == 0 )); then
  exit 1
fi
