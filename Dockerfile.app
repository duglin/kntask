FROM ubuntu
COPY /app /doit
ENTRYPOINT [ "/doit" ]
