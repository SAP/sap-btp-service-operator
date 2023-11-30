package ioutils_test

import (
	"errors"
	"fmt"
	"io"

	"github.com/SAP/sap-btp-service-operator/internal/ioutils"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LimitedWriter", func() {

	var SUT *ioutils.LimitedWriter

	var W *MockWriter // the SUT.W
	var N int64       // the initial SUT.N

	BeforeEach(func() {
		mockCtrl := gomock.NewController(GinkgoT())
		DeferCleanup(mockCtrl.Finish)

		W = NewMockWriter(mockCtrl)
		N = 0
	})

	JustBeforeEach(func() {
		SUT = &ioutils.LimitedWriter{W: W, N: N}
	})

	{
		reuseSpec := func() {
			DescribeTable(
				"Write",

				Entry("nil slice", nil),
				Entry("zero byte slice", []byte("")),
				Entry("one byte slice", []byte("1")),
				Entry("3 bytes slice", []byte("123")),

				func(p []byte) {
					// W not called

					// EXERCISE
					written, err := SUT.Write(p)

					// VERIFY
					Expect(written).To(Equal(0))
					Expect(err).To(BeIdenticalTo(ioutils.ErrLimitExceeded))
				},
			)
		}

		When("N == -1", func() {

			BeforeEach(func() {
				N = -1
			})

			reuseSpec()
		})

		When("N == 0", func() {

			BeforeEach(func() {
				N = -1
			})

			reuseSpec()
		})
	}

	customError1 := errors.New("customError1")

	type TestCase struct {
		p                 []byte
		expectWCalledWith []byte
		bytesWrittenByW   int
		errorFromW        error
		wantBytesWritten  int
		wantError         error
		wantN             int64
	}

	testFunc := func(tc TestCase) {
		Expect(SUT.W).To(BeIdenticalTo(W))

		W.EXPECT().
			Write(tc.expectWCalledWith).
			Return(tc.bytesWrittenByW, tc.errorFromW)

		// EXERCISE
		written, err := SUT.Write(tc.p)

		// VERIFY
		Expect(written).To(Equal(tc.wantBytesWritten))
		if tc.wantError != nil {
			Expect(err).To(BeIdenticalTo(tc.wantError))
		} else {
			Expect(err).ShouldNot(HaveOccurred())
		}
		Expect(SUT.N).To(Equal(tc.wantN))
		Expect(SUT.W).To(BeIdenticalTo(W))
	}

	entryDescriptionFunc := func(tc TestCase) string {
		return fmt.Sprintf(
			"when W.Write(p) == (%v, %v)",
			tc.bytesWrittenByW, tc.errorFromW,
		)
	}

	When("N == 1", func() {

		BeforeEach(func() {
			N = 1
		})

		// When len(p) == 0
		{
			reuseEntries := []TableEntry{
				// W writes 0 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        nil,
					wantN:            1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            1,
				}),

				// W writes 1 byte (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       nil,
					wantBytesWritten: 1,
					wantError:        nil,
					wantN:            0,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       customError1,
					wantBytesWritten: 1,
					wantError:        customError1,
					wantN:            0,
				}),

				// W writes 2 byte (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       nil,
					wantBytesWritten: 2,
					wantError:        nil,
					wantN:            -1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       customError1,
					wantBytesWritten: 2,
					wantError:        customError1,
					wantN:            -1,
				}),

				// W writes -1 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        nil,
					wantN:            1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            1,
				}),
			}

			When("p == <nil>", func() {

				DescribeTable(
					"Write",

					func(tc TestCase) {
						tc.p = nil
						tc.expectWCalledWith = nil
						testFunc(tc)
					},

					entryDescriptionFunc,
					reuseEntries,
				)
			})

			When("p == []byte{}", func() {

				DescribeTable(
					"Write",

					func(tc TestCase) {
						tc.p = []byte{}
						tc.expectWCalledWith = []byte{}
						testFunc(tc)
					},

					entryDescriptionFunc,
					reuseEntries,
				)
			})
		}

		When("len(p) == 1", func() {

			DescribeTable(
				"Write",

				func(tc TestCase) {
					tc.p = []byte("1")
					tc.expectWCalledWith = []byte("1")
					testFunc(tc)
				},

				entryDescriptionFunc,

				// W writes 0 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            1,
				}),

				// W writes 1 byte
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       nil,
					wantBytesWritten: 1,
					wantError:        nil,
					wantN:            0,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       customError1,
					wantBytesWritten: 1,
					wantError:        customError1,
					wantN:            0,
				}),

				// W writes 2 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       nil,
					wantBytesWritten: 2,
					wantError:        nil,
					wantN:            -1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       customError1,
					wantBytesWritten: 2,
					wantError:        customError1,
					wantN:            -1,
				}),

				// W writes 100 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       nil,
					wantBytesWritten: 100,
					wantError:        nil,
					wantN:            -99,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       customError1,
					wantBytesWritten: 100,
					wantError:        customError1,
					wantN:            -99,
				}),

				// W writes -1 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            1,
				}),
			)
		})

		When("len(p) == 10", func() {

			DescribeTable(
				"Write",

				func(tc TestCase) {
					tc.p = []byte("1234567890")
					tc.expectWCalledWith = []byte("1") // 1 byte
					testFunc(tc)
				},

				entryDescriptionFunc,

				// W writes 0 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            1,
				}),

				// W writes 1 byte
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       nil,
					wantBytesWritten: 1,
					wantError:        ioutils.ErrLimitExceeded,
					wantN:            0,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       customError1,
					wantBytesWritten: 1,
					wantError:        customError1,
					wantN:            0,
				}),

				// W writes 2 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       nil,
					wantBytesWritten: 2,
					wantError:        ioutils.ErrLimitExceeded,
					wantN:            -1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       customError1,
					wantBytesWritten: 2,
					wantError:        customError1,
					wantN:            -1,
				}),

				// W writes -1 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            1,
				}),
			)
		})
	})

	When("N == 10", func() {

		BeforeEach(func() {
			N = 10
		})

		// When len(p) == 0
		{
			reuseEntries := []TableEntry{
				// W writes 0 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        nil,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),

				// W writes 1 byte (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       nil,
					wantBytesWritten: 1,
					wantError:        nil,
					wantN:            9,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       customError1,
					wantBytesWritten: 1,
					wantError:        customError1,
					wantN:            9,
				}),

				// W writes 11 byte (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  11,
					errorFromW:       nil,
					wantBytesWritten: 11,
					wantError:        nil,
					wantN:            -1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  11,
					errorFromW:       customError1,
					wantBytesWritten: 11,
					wantError:        customError1,
					wantN:            -1,
				}),

				// W writes -1 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        nil,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),
			}

			When("p == <nil>", func() {

				DescribeTable(
					"Write",

					func(tc TestCase) {
						tc.p = nil
						tc.expectWCalledWith = nil
						testFunc(tc)
					},

					entryDescriptionFunc,
					reuseEntries,
				)
			})

			When("p == []byte{}", func() {

				DescribeTable(
					"Write",

					func(tc TestCase) {
						tc.p = []byte{}
						tc.expectWCalledWith = []byte{}
						testFunc(tc)
					},

					entryDescriptionFunc,
					reuseEntries,
				)
			})
		}

		When("len(p) == 1", func() {

			DescribeTable(
				"Write",

				func(tc TestCase) {
					tc.p = []byte("1")
					tc.expectWCalledWith = []byte("1")
					testFunc(tc)
				},

				entryDescriptionFunc,

				// W writes 0 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),

				// W writes 1 byte
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       nil,
					wantBytesWritten: 1,
					wantError:        nil,
					wantN:            9,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       customError1,
					wantBytesWritten: 1,
					wantError:        customError1,
					wantN:            9,
				}),

				// W writes 2 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       nil,
					wantBytesWritten: 2,
					wantError:        nil,
					wantN:            8,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  2,
					errorFromW:       customError1,
					wantBytesWritten: 2,
					wantError:        customError1,
					wantN:            8,
				}),

				// W writes 100 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       nil,
					wantBytesWritten: 100,
					wantError:        nil,
					wantN:            -90,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       customError1,
					wantBytesWritten: 100,
					wantError:        customError1,
					wantN:            -90,
				}),

				// W writes -1 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),
			)
		})

		When("len(p) == 10", func() {

			DescribeTable(
				"Write",

				func(tc TestCase) {
					tc.p = []byte("1234567890")
					tc.expectWCalledWith = []byte("1234567890")
					testFunc(tc)
				},

				entryDescriptionFunc,

				// W writes 0 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),

				// W writes 1 byte
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       nil,
					wantBytesWritten: 1,
					wantError:        io.ErrShortWrite,
					wantN:            9,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  1,
					errorFromW:       customError1,
					wantBytesWritten: 1,
					wantError:        customError1,
					wantN:            9,
				}),

				// W writes 9 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  9,
					errorFromW:       nil,
					wantBytesWritten: 9,
					wantError:        io.ErrShortWrite,
					wantN:            1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  9,
					errorFromW:       customError1,
					wantBytesWritten: 9,
					wantError:        customError1,
					wantN:            1,
				}),

				// W writes 10 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  10,
					errorFromW:       nil,
					wantBytesWritten: 10,
					wantError:        nil,
					wantN:            0,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  10,
					errorFromW:       customError1,
					wantBytesWritten: 10,
					wantError:        customError1,
					wantN:            0,
				}),

				// W writes 100 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       nil,
					wantBytesWritten: 100,
					wantError:        nil,
					wantN:            -90,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       customError1,
					wantBytesWritten: 100,
					wantError:        customError1,
					wantN:            -90,
				}),

				// W writes -1 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),
			)
		})

		When("len(p) == 11", func() {

			DescribeTable(
				"Write",

				func(tc TestCase) {
					tc.p = []byte("12345678901")                // 11 bytes
					tc.expectWCalledWith = []byte("1234567890") // 10 (== N) bytes
					testFunc(tc)
				},

				entryDescriptionFunc,

				// W writes 0 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  0,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),

				// W writes 10 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  10,
					errorFromW:       nil,
					wantBytesWritten: 10,
					wantError:        ioutils.ErrLimitExceeded,
					wantN:            0,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  10,
					errorFromW:       customError1,
					wantBytesWritten: 10,
					wantError:        customError1,
					wantN:            0,
				}),

				// W writes 11 bytes
				Entry(nil, TestCase{
					bytesWrittenByW:  11,
					errorFromW:       nil,
					wantBytesWritten: 11,
					wantError:        ioutils.ErrLimitExceeded,
					wantN:            -1,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  11,
					errorFromW:       customError1,
					wantBytesWritten: 11,
					wantError:        customError1,
					wantN:            -1,
				}),

				// W writes 100 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       nil,
					wantBytesWritten: 100,
					wantError:        ioutils.ErrLimitExceeded,
					wantN:            -90,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  100,
					errorFromW:       customError1,
					wantBytesWritten: 100,
					wantError:        customError1,
					wantN:            -90,
				}),

				// W writes -1 bytes (off-spec)
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       nil,
					wantBytesWritten: 0,
					wantError:        io.ErrShortWrite,
					wantN:            10,
				}),
				Entry(nil, TestCase{
					bytesWrittenByW:  -1,
					errorFromW:       customError1,
					wantBytesWritten: 0,
					wantError:        customError1,
					wantN:            10,
				}),
			)
		})
	})
})
