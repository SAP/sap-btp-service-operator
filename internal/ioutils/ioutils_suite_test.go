package ioutils

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIOUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "IOUtils")
}
