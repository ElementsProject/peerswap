FROM golang:1.17-bullseye as builder

# Create a directory for the build
WORKDIR /peerswap

# Copy the current host peerswap directory into the container
COPY . .

# Build
# We don't need the c-lightning plugin in this container,
# because for c-lightning, the plugin needs to be in the same
# container as c-lightning itself
RUN make -j$(nproc) lnd-release

FROM debian:bullseye-slim

# Copy built binaries
COPY --from=builder /peerswap/peerswapd /usr/bin
COPY --from=builder /peerswap/pscli /usr/bin

CMD ["/usr/bin/peerswapd"]
