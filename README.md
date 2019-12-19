TODOs:
- add batching controller/workflow on top
- get CMD/ENTRYPOINT from image so we can use it instead of /app
- report status to batching controller so it can retry or know when it's done
- what do we need to buffer so we can try?

https://cloud.ibm.com/docs/containers?topic=containers-file_storage
https://cloud.ibm.com/docs/containers?topic=containers-storage_planning#choose_storage_solution
https://docs.microsoft.com/en-us/azure/batch/tutorial-rendering-cli
https://ibm.ent.box.com/notes/573702339552

controller (just PoC)
- start job
- query status of job
