# Substitute GOBIN for your bin directory
# Leave unset to default to $GOPATH/bin
GO111MODULE=on GOBIN=/usr/local/bin go install \
github.com/bufbuild/buf/cmd/buf@v1.30.1

# install templ
go install github.com/a-h/templ/cmd/templ@latest

sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d

apt update
apt install -y npm
npm install -D tailwindcss
npx tailwindcss init
