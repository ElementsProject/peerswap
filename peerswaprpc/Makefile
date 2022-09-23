DIR=peerswaprpc

PROTOC_OPTS= \
		-I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--grpc-gateway_out=. \
    	--grpc-gateway_opt logtostderr=true \
    	--grpc-gateway_opt paths=source_relative \
		--grpc-gateway_opt grpc_api_configuration=${DIR}/peerswap.yaml

PROTOC_SWAGGER_OPTS= \
		--openapiv2_out=. \
		--openapiv2_opt logtostderr=true \
		--openapiv2_opt grpc_api_configuration=${DIR}/peerswap.yaml

STUBS= \
		${DIR}/peerswaprpc_grpc.pb.go \
		${DIR}/peerswaprpc.pb.go \
		${DIR}/peerswaprpc.pb.gw.go

all-rpc: ${STUBS} ${DIR}/peerswaprpc.swagger.json

clean-rpc:
		rm -f ${STUBS} ${DIR}/peerswaprpc.swagger.json

${DIR}/peerswaprpc.swagger.json:
	protoc ${PROTOC_SWAGGER_OPTS} ${DIR}/peerswaprpc.proto

${DIR}/peerswaprpc_grpc.pb.go ${DIR}/peerswaprpc.pb.go ${DIR}/peerswaprpc.pb.gw.go: ${DIR}/peerswaprpc.proto
	protoc ${PROTOC_OPTS} ${DIR}/peerswaprpc.proto
PHONY:
