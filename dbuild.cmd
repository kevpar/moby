docker run -m 4gb --name binaries -e DOCKER_GITCOMMIT=$DOCKER_GITCOMMIT nativebuildimage hack\make.ps1 -Binary
docker cp binaries:C:\go\src\github.com\docker\docker\bundles\docker.exe /docker
docker cp binaries:C:\go\src\github.com\docker\docker\bundles\dockerd.exe /docker
docker rm binaries