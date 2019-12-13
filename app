#!/bin/bash

sleep 1
echo $(date) - In app  - Index: ${KN_JOB_INDEX}

# Fail 1/3 of the time
if (( $RANDOM % 3 == 0 )); then
  exit 1
fi
