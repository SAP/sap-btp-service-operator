package ioutils_test

import (
	"errors"

	"github.com/SAP/sap-btp-service-operator/internal/ioutils"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ErrorConversionWriter", func() {

	var SUT *ioutils.ErrorConversionWriter

	var W *MockWriter // the SUT.W

	BeforeEach(func() {
		mockCtrl := gomock.NewController(GinkgoT())
		DeferCleanup(mockCtrl.Finish)

		W = NewMockWriter(mockCtrl)
	})

	JustBeforeEach(func() {
		SUT = &ioutils.ErrorConversionWriter{W: W} // Converter not set yet
	})

	It("panicks if Converter is nil", func() {

		Expect(SUT.Converter).To(BeNil())

		input := []byte("A")

		W.EXPECT().
			Write(input).
			Return(0, errors.New("error1"))

		exercise := func() {
			// EXERCISE
			SUT.Write(input)
		}

		// VERIFY
		Expect(exercise).To(Panic())
	})

	It("calls Converter if W returns error", func() {
		input := []byte("A")
		bytesWritterFromW := 123
		errorFromW := errors.New("errorFromW")
		errorFromConverter := errors.New("errorFromConverter")

		W.EXPECT().
			Write(input).
			Return(bytesWritterFromW, errorFromW)

		SUT.Converter = func(err error) error {
			Expect(err).To(BeIdenticalTo(errorFromW))
			return errorFromConverter
		}

		// EXERCISE
		bytesWritten, err := SUT.Write(input)

		// VERIFY
		Expect(err).To(BeIdenticalTo(errorFromConverter))
		Expect(bytesWritten).To(Equal(bytesWritterFromW))
	})

	It("does not call Converter if W returns no error", func() {
		input := []byte("A")
		bytesWritterFromW := 9991

		W.EXPECT().
			Write(input).
			Return(bytesWritterFromW, nil)

		SUT.Converter = func(err error) error {
			Fail("Converter called unexpectedly")
			return err
		}

		// EXERCISE
		bytesWritten, err := SUT.Write(input)

		// VERIFY
		Expect(err).ShouldNot(HaveOccurred())
		Expect(bytesWritten).To(Equal(bytesWritterFromW))
	})
})
