#!/bin/bash

(

# If we're streaming then just echo whatever comes across stdin, but reversed
if [[ "$K_STREAM" == "true" ]]; then
  while read line ; do
    echo "$line" | rev
  done
  # rev
  echo exit streaming 
  # echo "In '$0' (host:`hostname`) ${K_JOB_INDEX:+(Index:$K_JOB_INDEX) }args: $*"
  exit 0
fi

# We're not streaming so show env vars
echo "In '$0' (host:$(hostname)) ${K_JOB_INDEX:+(Index:$K_JOB_INDEX) }args: $*"
# env | sort | grep -v -e JOBCONTROLLER -e ^PULL -e ^TEST -e KUBE -e K_HEADERS
env | sort | grep -e ^K_  | grep -v K_HEADERS

if [[ "$K_TYPE" == "pull" ]]; then
	# Event pulling app so stdin is the event, just echo it
	echo "The app saw:"
	cat
	exit 0
fi

# Sleep and randomly exit with non-0
echo -e sleeping ${SLEEP:-1}\\n
sleep ${SLEEP:-1}

# Fail 1/3 of the time if this is run as a batch job
if [[ -z "$PASS" ]] && [[ -n "$K_JOB_NAME" ]] && (( $RANDOM % 3 == 0 )) ; then
  exit 1
fi

echo Body is...
cat

) | tee /dev/fd/2
