DIR=peerswaprpc

STUBS= \
		${DIR}/peerswaprpc_grpc.pb.go \
		${DIR}/peerswaprpc.pb.go \
		${DIR}/peerswaprpc.pb.gw.go \
		${DIR}/peerswaprpc.swagger.json

all-rpc:
		buf generate
.PHONY: all-rpc

buf-lint:
		buf lint
.PHONY: all-rpc

buf-format:
		buf format -w
.PHONY: all-rpc

clean-rpc:
		rm -f ${STUBS}

all-rpc-docker:
	./gen_protos_docker.sh
.PHONY: all-rpc-docker
