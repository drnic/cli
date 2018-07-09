package ccv2_test

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/cli/api/cloudcontroller/ccerror"
	. "code.cloudfoundry.org/cli/api/cloudcontroller/ccv2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/ghttp"
)

var _ = Describe("Buildpack", func() {
	var client *Client

	BeforeEach(func() {
		client = NewTestClient()
	})

	Describe("CreateBuildpack", func() {
		var (
			buildpack  Buildpack
			warnings   Warnings
			executeErr error
		)

		JustBeforeEach(func() {
			buildpack, warnings, executeErr = client.CreateBuildpack(Buildpack{
				Name:     "potato",
				Position: 1,
				Enabled:  true,
			})
		})

		Context("when the creation is successful", func() {
			BeforeEach(func() {
				response := `
				{
					"metadata": {
						"guid": "some-guid"
					},
					"entity": {
						"name": "potato",
						"stack": "null",
						"position": 1,
						"enabled": true
					}
				}`
				requestBody := map[string]interface{}{
					"name":     "potato",
					"position": 1,
					"enabled":  true,
				}
				server.AppendHandlers(
					CombineHandlers(
						VerifyRequest(http.MethodPost, "/v2/buildpacks"),
						VerifyJSONRepresenting(requestBody),
						RespondWith(http.StatusCreated, response, http.Header{"X-Cf-Warnings": {"this is a warning"}}),
					),
				)
			})

			It("returns a buildpack and any warnings", func() {
				Expect(server.ReceivedRequests()).To(HaveLen(2))

				Expect(executeErr).ToNot(HaveOccurred())
				Expect(buildpack).To(Equal(Buildpack{
					GUID:     "some-guid",
					Name:     "potato",
					Enabled:  true,
					Position: 1,
				}))
				Expect(warnings).To(ConsistOf(Warnings{"this is a warning"}))
			})
		})

		Context("when the create returns an error", func() {
			BeforeEach(func() {
				response := `
					{
						"description": "Request invalid due to parse error: Field: name, Error: Missing field name",
						"error_code": "CF-MessageParseError",
						"code": 1001
					}
				`
				server.AppendHandlers(
					CombineHandlers(
						VerifyRequest(http.MethodPost, "/v2/buildpacks"),
						RespondWith(http.StatusBadRequest, response, http.Header{"X-Cf-Warnings": {"this is a warning"}}),
					),
				)
			})

			It("returns the error and warnings", func() {
				Expect(executeErr).To(MatchError(ccerror.BadRequestError{Message: "Request invalid due to parse error: Field: name, Error: Missing field name"}))
				Expect(warnings).To(ConsistOf(Warnings{"this is a warning"}))
			})
		})
	})

	FDescribe("UploadBuildpack", func() {
		var (
			warnings   Warnings
			executeErr error
			bpFile     *os.File
			bpFilePath string

			err error
		)

		BeforeEach(func() {
			bpFile, err = ioutil.TempFile("", "example-bp.zip")
			Expect(err).ToNot(HaveOccurred())
			// Expect(bpFile.Close()).ToNot(HaveOccurred())
			Expect(os.RemoveAll(bpFile.Name())).ToNot(HaveOccurred())

			dir, err := ioutil.TempDir("", "buildpack-dir-")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)

			bpFilePath = filepath.Join(dir, bpFile.Name())
		})

		JustBeforeEach(func() {
			warnings, executeErr = client.UploadBuildpack("some-buildpack-guid", bpFilePath, bpFile, int64(6))
		})

		Context("when the upload is successful", func() {
			BeforeEach(func() {
				response := `{
					"metadata": {
						"guid": "buildpack-guid",
						"url": "/v2/buildpacks/buildpack-guid/bits"
					},
					"entity": {
						"guid": "buildpack-guid",
						"status": "queued"
					}
				}`

				server.AppendHandlers(
					CombineHandlers(
						VerifyRequest(http.MethodPut, "/v2/buildpacks/some-buildpack-guid/bits"),
						RespondWith(http.StatusOK, response, http.Header{"X-Cf-Warnings": {"this is a warning"}}),
					),
				)
			})

			It("returns warnings", func() {
				Expect(warnings).To(ConsistOf(Warnings{"this is a warning"}))
				Expect(executeErr).ToNot(HaveOccurred())
			})
		})

		Context("when the upload returns an error", func() {
			BeforeEach(func() {
				response := `{
					"code": 30003,
					"description": "The buildpack could not be found: some-buildpack-guid",
					"error_code": "CF-Banana"
				}`

				server.AppendHandlers(
					CombineHandlers(
						VerifyRequest(http.MethodPut, "/v2/buildpacks/some-buildpack-guid/bits"),
						RespondWith(http.StatusNotFound, response, http.Header{"X-Cf-Warnings": {"this is a warning"}}),
					),
				)
			})

			It("returns the error and warnings", func() {
				Expect(executeErr).To(MatchError(ccerror.ResourceNotFoundError{Message: "The buildpack could not be found: some-buildpack-guid"}))
				Expect(warnings).To(ConsistOf(Warnings{"this is a warning"}))
			})
		})
	})

})
