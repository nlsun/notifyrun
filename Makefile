rel: rel-linux rel-darwin

rel-linux:
	GOOS=linux GOARCH=amd64 \
		 go build -o notifyrun_linux_amd64 \
		 github.com/nlsun/notifyrun/pkg/cmd/notifyrun

rel-darwin:
	GOOS=darwin GOARCH=amd64 \
		 go build -o notifyrun_darwin_amd64 \
		 github.com/nlsun/notifyrun/pkg/cmd/notifyrun
