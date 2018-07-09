package ccv2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"

	"code.cloudfoundry.org/cli/api/cloudcontroller"
	"code.cloudfoundry.org/cli/api/cloudcontroller/ccv2/internal"
)

// Buildpack represents a Cloud Controller Buildpack.
type Buildpack struct {
	Enabled  bool   `json:"enabled,omitempty"`
	GUID     string `json:"guid,omitempty"`
	Name     string `json:"name"`
	Position int    `json:"position,omitempty"`
}

func (buildpack *Buildpack) UnmarshalJSON(data []byte) error {
	var alias struct {
		Metadata struct {
			GUID string `json:"guid"`
		} `json:"metadata"`
		Entity struct {
			Name     string `json:"name"`
			Position int    `json:"position"`
			Enabled  bool   `json:"enabled"`
		} `json:"entity"`
	}
	err := json.Unmarshal(data, &alias)
	if err != nil {
		return err
	}

	buildpack.Enabled = alias.Entity.Enabled
	buildpack.GUID = alias.Metadata.GUID
	buildpack.Name = alias.Entity.Name
	buildpack.Position = alias.Entity.Position

	return nil
}

func (client *Client) CreateBuildpack(buildpack Buildpack) (Buildpack, Warnings, error) {
	body, err := json.Marshal(buildpack)
	if err != nil {
		return Buildpack{}, nil, err
	}

	request, err := client.newHTTPRequest(requestOptions{
		RequestName: internal.PostBuildpackRequest,
		Body:        bytes.NewReader(body),
	})
	if err != nil {
		return Buildpack{}, nil, err
	}

	var createdBuildpack Buildpack
	response := cloudcontroller.Response{
		Result: &createdBuildpack,
	}

	err = client.connection.Make(request, &response)
	return createdBuildpack, response.Warnings, err
}

func (client *Client) UploadBuildpack(buildpackGUID string, buildpackPath string, buildpack io.Reader, buildpackLength int64) (Warnings, error) {

	contentType, body, writeErrors := client.createMultipartBodyAndHeaderForBuildpack(buildpack, buildpackPath)

	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	s := buf.String()

	fmt.Printf("body s, %v", s)
	// fmt.Printf("writeErrors, %v", writeErrors)

	request, err := client.newHTTPRequest(requestOptions{
		RequestName: internal.PutBuildpackRequest,
		URIParams:   Params{"buildpack_guid": buildpackGUID},
		Body:        body,
	})

	if err != nil {
		fmt.Printf("err from generating requst, %v", request)
		return nil, err
	}

	request.Header.Set("Content-Type", contentType)

	_, warnings, err := client.uploadBuildpackAsynchronously(request, writeErrors)
	if err != nil {
		return warnings, err
	}
	return warnings, nil

}

func (*Client) createMultipartBodyAndHeaderForBuildpack(buildpack io.Reader, bpPath string) (string, io.ReadSeeker, <-chan error) {
	writerOutput, writerInput := cloudcontroller.NewPipeBomb()

	form := multipart.NewWriter(writerInput)

	writeErrors := make(chan error)

	go func() {
		defer close(writeErrors)
		defer writerInput.Close()

		bpFileName := filepath.Base(bpPath) // was passing bpPath necessary here?
		writer, err := form.CreateFormFile("buildpack", bpFileName)
		if err != nil {
			writeErrors <- err
			fmt.Printf("error from createformfile, %v", writeErrors)
			return
		}

		_, err = io.Copy(writer, buildpack)
		if err != nil {
			writeErrors <- err
			fmt.Printf("error from copy, %v", writeErrors)
			return
		}

		err = form.Close()
		if err != nil {
			writeErrors <- err
		}
	}()

	return form.FormDataContentType(), writerOutput, writeErrors
}

func (client *Client) uploadBuildpackAsynchronously(request *cloudcontroller.Request, writeErrors <-chan error) (Buildpack, Warnings, error) {

	var buildpack Buildpack
	response := cloudcontroller.Response{
		Result: &buildpack,
	}

	httpErrors := make(chan error)

	go func() {
		defer close(httpErrors)

		err := client.connection.Make(request, &response)
		if err != nil {
			httpErrors <- err
		}
	}()

	// The following section makes the following assumptions:
	// 1) If an error occurs during file reading, an EOF is sent to the request
	// object. Thus ending the request transfer.
	// 2) If an error occurs during request transfer, an EOF is sent to the pipe.
	// Thus ending the writing routine.
	var firstError error
	var writeClosed, httpClosed bool

	for {
		select {
		case writeErr, ok := <-writeErrors:
			// fmt.Printf("writeerr hit")
			if !ok {
				writeClosed = true
				break // for select
			}
			if firstError == nil {
				firstError = writeErr
			}
		case httpErr, ok := <-httpErrors:
			fmt.Printf("httperr hit")
			if !ok {
				httpClosed = true
				break // for select
			}
			if firstError == nil {
				firstError = httpErr
			}
		}

		if writeClosed && httpClosed {
			break // for for
		}
	}

	fmt.Printf("response %v", response)
	return buildpack, response.Warnings, firstError
}
