DIR=peerswaprpc

STUBS= \
		${DIR}/peerswaprpc_grpc.pb.go \
		${DIR}/peerswaprpc.pb.go \
		${DIR}/peerswaprpc.pb.gw.go \
		${DIR}/peerswaprpc.swagger.json

all-rpc:
		buf generate

clean-rpc:
		rm -f ${STUBS}

.PHONY: all-rpc-docker
all-rpc-docker:
	./gen_protos_docker.sh

