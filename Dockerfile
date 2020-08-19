FROM registry.access.redhat.com/ubi8/ubi:latest AS builder

ARG GO_PACKAGE_PATH=github.com/jenkinsci/jenkins-operator

# Basics
ENV GIT_COMMITTER_NAME="OpenShift Developer Sevices"
ENV GIT_COMMITTER_EMAIL=openshift-dev-services+jenkins@redhat.com
ENV LANG=en_US.utf8

# Go
ENV GOPATH=/tmp/go
ENV GOROOT=/tmp/goroot/go
ENV GO_VERSION=1.14.1
ENV GO_DOWNLOAD_SITE=https://dl.google.com/go/
ENV GO_DIST=go$GO_VERSION.linux-amd64.tar.gz

# Operator SDK
ENV OPERATOR_SDK_VERSION=v0.17.0
ENV OPERATOR_SDK_DOWNLOAD_SITE=https://github.com/operator-framework/operator-sdk/releases/download/
ENV OPERATOR_SDK_DIST=operator-sdk-$OPERATOR_SDK_VERSION-x86_64-linux-gnu

# kubectl
ENV KUBECTL_VERSION=v1.14.3
ENV KUBECTL_DOWNLOAD_SITE=https://storage.googleapis.com/kubernetes-release/release/
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin

WORKDIR /tmp
RUN mkdir -p $GOPATH/bin &&  \
    mkdir -p /tmp/goroot && \
    curl -Lo $GO_DIST $GO_DOWNLOAD_SITE/$GO_DIST && \
    tar -C /tmp/goroot -xzf $GO_DIST && \
    curl -Lo kubectl $KUBECTL_DOWNLOAD_SITE/$KUBECTL_VERSION/bin/linux/amd64/kubectl && \
    chmod +x kubectl && mv kubectl $GOPATH/bin/ && \
    curl -Lo operator-sdk $OPERATOR_SDK_DOWNLOAD_SITE/$OPERATOR_SDK_VERSION/$OPERATOR_SDK_DIST && \
    chmod +x operator-sdk && mv operator-sdk $GOPATH/bin/ && \
    mkdir -p ${GOPATH}/src/${GO_PACKAGE_PATH}/ && \
    yum -y install --nodocs git make gcc

WORKDIR ${GOPATH}/src/${GO_PACKAGE_PATH}

# Copy only relevant things (instead of all) to speed-up the build.
COPY assets assets
COPY build build
COPY cmd cmd
COPY deploy deploy
COPY pkg pkg
COPY version version
COPY test test
COPY internal internal
COPY go.mod go.sum LICENSE Makefile tools.go config.base.env config.minikube.env config.openshift.env VERSION.txt gen-crd-api-config.json ./

ARG VERBOSE=2
RUN make build

FROM registry.access.redhat.com/ubi8/ubi-minimal

LABEL com.redhat.delivery.operator.bundle=true
LABEL com.redhat.openshift.versions="v4.5"
LABEL com.redhat.delivery.backport=true

LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=jenkins-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.bundle.channel.default.v1=alpha
LABEL maintainer "openshift-dev-services+jenkins@redhat.com" \
      version="0.5.0" \
      io.k8s.display.name="jenkins operator" \
      io.k8s.description="jenkins operator is a Kubernetes native operator which fully manages Jenkins on Kubernetes"
ENV LANG=en_US.utf8
ENV OPERATOR_NAME=jenkins-operator
ENV OPERATOR=/usr/local/bin/$OPERATOR_NAME
ENV GOPATH=/tmp/go


COPY deploy/olm-catalog/ /manifests/
COPY deploy/metadata/annotations.yaml /metadata/annotations.yaml
COPY --from=builder $GOPATH/src/github.com/jenkinsci/jenkins-operator/build/_output/bin/${OPERATOR_NAME} ${OPERATOR}

ENTRYPOINT [ "/usr/local/bin/jenkins-operator" ]
