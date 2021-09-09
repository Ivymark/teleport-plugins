FROM quay.io/gravitational/teleport-buildbox:go1.16.2

WORKDIR app
COPY . .
RUN go build -o teleport-slack github.com/gravitational/teleport-plugins/access/slack

ENTRYPOINT ["/app/teleport-slack"]
CMD ["start"]
