os=$(go env GOOS)
arch=$(go env GOARCH)

# download kubebuilder and extract it to tmp
#curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.7.0/kubebuilder_${os}_${arch}.tar.gz | tar -xz -C /tmp/
curl -L -o kubebuilder https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.7.0/kubebuilder_${os}_${arch}
# move to a long-term location and put it on your path
# (you'll need to set the KUBEBUILDER_ASSETS env var if you put it somewhere else)
sudo chmod +x kubebuilder
sudo mv kubebuilder /usr/local/kubebuilder

ETCD_VER=v3.5.6
curl -L https://github.com/etcd-io/etcd/releases/download/${ETCD_VER}/etcd-${ETCD_VER}-${os}-${arch}.tar.gz -o /tmp/etcd-${ETCD_VER}-${os}-${arch}.tar.gz
tar xzvf /tmp/etcd-${ETCD_VER}-${os}-${arch}.tar.gz -C /usr/local/bin --strip-components=1
rm -f /tmp/etcd-${ETCD_VER}-${os}-${arch}.tar.gz

export TEST_ASSET_ETCD=/usr/local/bin