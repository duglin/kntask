FROM ubuntu
COPY /app /doit
# CMD [ "arg1" ]
ENTRYPOINT [ "/doit" ]
