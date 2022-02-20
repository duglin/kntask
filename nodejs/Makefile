all: .func-node

.func-node: main.js app.js Dockerfile
	docker build -t duglin/func-node .
	docker push duglin/func-node
	touch .func-node
