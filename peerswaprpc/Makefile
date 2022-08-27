PROTOC_OPTS= \
		-I. \
		--go_out=paths=source_relative:. \
		--go-grpc_out=paths=source_relative:. \
		--grpc-gateway_out=. \
    	--grpc-gateway_opt logtostderr=true \
    	--grpc-gateway_opt paths=source_relative \
    	--grpc-gateway_opt generate_unbound_methods=true \
		--grpc-gateway_opt grpc_api_configuration=./peerswap.yaml \
		--openapiv2_out=. \
    	--openapiv2_opt logtostderr=true

STUBS= \
		peerswaprpc_grpc.pb.go \
		peerswaprpc.pb.go \
		peerswaprpc.pb.gw.go \
		peerswaprpc.swagger.json

all-rpc: ${STUBS}

clean-rpc:
		rm -f ${STUBS}

peerswaprpc_grpc.pb.go peerswaprpc.pb.go peerswaprpc.pb.gw.go peerswaprpc.swagger.json: peerswaprpc.proto
	protoc ${PROTOC_OPTS} peerswaprpc.proto


