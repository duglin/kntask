all: .taskmgr .app .jobcontroller load kn-job

taskmgr: taskmgr.go
	GO_EXTLINK_ENABLED=0 CGO_ENABLED=0 go build \
   	-ldflags "-s -w -extldflags -static" \
	-tags netgo -installsuffix netgo \
	-o taskmgr taskmgr.go

.taskmgr: taskmgr.go Dockerfile.taskmgr
	go build -o /dev/null taskmgr.go # quick fail
	docker build -f Dockerfile.taskmgr -t duglin/taskmgr .
	docker push duglin/taskmgr
	touch .taskmgr

.jobcontroller: jobcontroller.go Dockerfile.jobcontroller
	go build -o /dev/null jobcontroller.go # quick fail
	docker build -f Dockerfile.jobcontroller -t duglin/jobcontroller .
	docker push duglin/jobcontroller
	touch .jobcontroller

.app: app Dockerfile.app .taskmgr
	docker build -f Dockerfile.app -t duglin/app .
	docker push duglin/app
	touch .app

run: .app
	docker run -ti -p 8080:8080 duglin/app

deploy: .jobcontroller .taskmgr .app
	kn service delete test jobcontroller > /dev/null 2>&1 || true
	sleep 2
	kn service create jobcontroller --image duglin/jobcontroller --min-scale=1
	./prep
	kubectl create -f s.yaml > /dev/null 2>&1

load: load.go
	go build -o load load.go
	GOOS=darwin GOARCH=amd64 go build -o load.mac load.go

kn-job: kn-job.go
	go build -o kn-job kn-job.go
	GOOS=darwin GOARCH=amd64 go build -o kn-job.mac kn-job.go

clean:
	rm -f jobcontroller load load.mac taskmgr kn-job kn-job.mac load.mac
