os=$(go env GOOS)
arch=$(go env GOARCH)

# download kubebuilder and extract it to tmp
#curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.7.0/kubebuilder_${os}_${arch}.tar.gz | tar -xz -C /tmp/
curl -k -L -s --compressed https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.7.0/kubebuilder_${os}_${arch}
# move to a long-term location and put it on your path
# (you'll need to set the KUBEBUILDER_ASSETS env var if you put it somewhere else)
sudo mv kubebuilder_${os}_${arch} /usr/local/kubebuilder
export PATH=$PATH:/usr/local/kubebuilder/bin
