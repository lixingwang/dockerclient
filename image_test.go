package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

var image *dockerImage

func init() {
	os.Setenv("DOCKER_API_VERSION", "1.24")
	client, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	image = NewDockerImage(client)
}
func TestListImages(t *testing.T) {
	os.Setenv("DOCKER_API_VERSION", "1.24")
	client, _ := client.NewEnvClient()
	image := NewDockerImage(client)
	sub, err := image.ListImages()
	assert.NotNil(t, sub)
	assert.Nil(t, err)
}

func TestImagePush(t *testing.T) {
	createTestImages("test_1")
	image.PushImages("test_1")
	image.DelImage("test_1")
}

func TestImageDelete(t *testing.T) {
	createTestImages("test_123")
	dels, err := image.DelImage("test_123")
	assert.NotNil(t, dels)
	assert.Nil(t, err)
}

func createTestImages(name string) error {
	var testDockerFile = `FROM scratch
MAINTAINER Tracy Li
EXPOSE 9090 9091
CMD ["/bin/true"]
`
	return image.BuildImage(testDockerFile, "./", "name")

}

func TestImageCreate(t *testing.T) {
	err := createTestImages("test_create")
	assert.Nil(t, err)
	_, err = image.DelImage("test_1")
	assert.Nil(t, err)

}

func TestContainerCreate(t *testing.T) {
	err := createTestImages("test_c_1")
	assert.Nil(t, err)

	config := container.Config{}
	config.Image = "test_c_1"

	id, err := image.CreateContainer("test-container", &config, nil)
	if err != nil {
		t.Error(err)
		t.Fatal(err)
	}

	err = image.RemoveContainer(id)
	if err != nil {
		t.Error(err)
		t.Fatal(err)
	}

	_, err = image.DelImage("test_c_1")
	assert.Nil(t, err)

	log.Debugf("Container id: %s", id)
}
