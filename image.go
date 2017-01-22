package main

import (
	"context"
	"encoding/json"
	"errors"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"
)

type dockerImage struct {
	client *client.Client
}

type imageName struct {
	Name           string
	DockerRegistry string
}

func (n imageName) GetFullImageName() string {
	if n.DockerRegistry != "" {
		if !strings.HasSuffix(n.DockerRegistry, "/") {
			n.DockerRegistry = n.DockerRegistry + "/"
		}

	}
	return n.DockerRegistry + n.Name
}

func NewImageName(name, dockerRegistry string) *imageName {
	return &imageName{Name: name, DockerRegistry: dockerRegistry}
}

func NewDockerImage(client *client.Client) *dockerImage {
	return &dockerImage{client: client}
}

func (d *dockerImage) ListImages() ([]types.ImageSummary, error) {
	imagesSummeries, err := d.client.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		return imagesSummeries, err
	}
	for _, summery := range imagesSummeries {
		//fmt.Println(summery.ID)
		log.Debug(summery.RepoTags)
	}
	return imagesSummeries, err
}

func (d *dockerImage) BuildImage(dockerfileContent string, enginePath string, dockerName string) error {
	err := ioutil.WriteFile(path.Join(enginePath, "Dockerfile"), []byte(dockerfileContent), 0644)
	if err != nil {
		return err
	}

	log.Debugf("Docker context dir: %s and engine path: %s", enginePath, enginePath)
	compression := archive.Uncompressed
	buildCtx, err := archive.TarWithOptions(enginePath, &archive.TarOptions{
		Compression:     compression,
		ExcludePatterns: []string{},
		IncludeFiles:    []string{},
	})
	if err != nil {
		log.Debugf("Archive build conext error %s", err.Error())
		return err
	}
	log.Debugf("Docker files: %s", "Dockerfile")
	buildResponse, err := d.client.ImageBuild(context.Background(), buildCtx, types.ImageBuildOptions{Tags: []string{dockerName}, Dockerfile: "Dockerfile"})
	if err != nil {
		log.Debugf("Build image error %s", err.Error())
		return err
	}
	log.Debugf("Build os type %s", buildResponse.OSType)
	b, err := ioutil.ReadAll(buildResponse.Body)
	if err != nil {
		log.Debugf("Read build response from read error %s", err.Error())
		return err
	}
	log.Debugf("Build image response %s", string(b))
	return nil
}

func (d *dockerImage) TagImage(repositoryName string, ref string) error {
	return d.client.ImageTag(context.Background(), repositoryName, ref)
}

func (d *dockerImage) PullImage(repositoryName string) error {
	options := types.ImagePullOptions{}
	reader, err := d.client.ImagePull(context.Background(), repositoryName, options)
	if err != nil {
		log.Errorf("Pull images error %s", err.Error())
		return err
	}

	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}
	log.Debugf("pull image response %s", string(b))
	defer reader.Close()
	return nil
}

func (d *dockerImage) PushImages(repositoryName string) error {
	buildResponse, err := d.client.ImagePush(context.Background(), repositoryName, types.ImagePushOptions{RegistryAuth: ""})
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(buildResponse)
	if err != nil {
		return err
	}
	log.Debugf("push image response %s", string(b))
	return nil
}

func (d *dockerImage) DelImage(repositoryName string) ([]types.ImageDelete, error) {
	client, err := client.NewEnvClient()
	dels, err := client.ImageRemove(context.Background(), repositoryName, types.ImageRemoveOptions{Force: true})
	log.Debugf("Delete image response %+v", dels)
	return dels, err
}

func (d *dockerImage) RunContainer(name imageName, containerName string, conf *container.Config, hostConf *container.HostConfig) (string, error) {
	config := container.Config{}
	config.Image = name.GetFullImageName()

	id, err := d.CreateContainer(containerName, conf, hostConf)
	if err != nil {
		return id, err
	}
	log.Debugf("Create container %s done", id)
	err = d.StartContainer(id)
	if err != nil {
		return "", err
	}
	log.Debugf("Start container %s done", id)

	return id, nil
}

func (d *dockerImage) InspectContainer(containerId string) (types.ContainerJSON, error) {
	result, err := d.client.ContainerInspect(context.Background(), containerId)
	if err != nil {
		log.Errorf("Inspect container error: %s", err.Error())
		return types.ContainerJSON{}, err
	}

	log.Debugf("status is: %s ", result.State.Status)
	return result, nil
}

func (d *dockerImage) StatusContainer(containerId string) (string, error) {
	result, err := d.client.ContainerInspect(context.Background(), containerId)
	if err != nil {
		log.Errorf("Inspect container error: %s", err.Error())
		return "", err
	}

	log.Debugf("status is: %s ", result.State.Status)
	return result.State.Status, nil
}

func (d *dockerImage) WaitContainerDone(containerId string) error {

	done := make(chan bool)
	cErr := make(chan error)

	go func() {
		status, err := d.StatusContainer(containerId)

		for {
			if err != nil {
				done <- true
				cErr <- err
			}

			log.Debugf("Container %s status %s", containerId, status)
			if strings.EqualFold("done", status) || strings.EqualFold("exited", status) {
				done <- true
				cErr <- nil
			} else {
				time.Sleep(1 * time.Second)
				status, err = d.StatusContainer(containerId)
			}
		}

	}()

	<-done
	err := <-cErr

	return err
}

func (d *dockerImage) StartContainer(containerId string) error {
	startOptions := types.ContainerStartOptions{}

	err := d.client.ContainerStart(context.Background(), containerId, startOptions)
	if err != nil {
		log.Errorf("Start container error: %s", err.Error())
		return err
	}
	return nil
}

func (d *dockerImage) ContainerLog(containerId string) (string, error) {
	logOptions := types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true}

	reader, err := d.client.ContainerLogs(context.Background(), containerId, logOptions)
	if err != nil {
		log.Errorf("Start container error: %s", err.Error())
		return "", err
	}
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		log.Errorf("Read container log from reader error: %s", err.Error())
		return "", err
	}
	log.Debugf(string(b))
	return string(b), nil
}

func (d *dockerImage) CreateContainer(containerName string, conf *container.Config, hostConf *container.HostConfig) (string, error) {
	res, err := d.client.ContainerCreate(context.Background(), conf, hostConf, nil, containerName)
	if err != nil {
		log.Errorf("Create container error: %s", err.Error())
		if strings.Contains(err.Error(), "No such image") {
			//Pull from registry
			log.Debugf("Pulling image: %s", conf.Image)
			err := d.PullImage(conf.Image)
			if err != nil {
				return "", err
			}
			s, err := d.CreateContainer(containerName, conf, hostConf)
			if err != nil {
				return "", err
			}
			return s, nil
		} else if strings.Contains(err.Error(), "is already in use by container") {
			log.Debugf("remove container name %s", containerName)
			err := d.RemoveContainer(containerName)
			if err != nil {
				log.Errorf("Remove container error")

			} else {
				s, err := d.CreateContainer(containerName, conf, hostConf)
				if err != nil {
					return "", err
				}
				return s, nil
			}

		} else {
			return "", err
		}

	}
	return res.ID, nil
}

func (d *dockerImage) RemoveContainer(containerId string) error {
	options := types.ContainerRemoveOptions{Force: true}
	err := d.client.ContainerRemove(context.Background(), containerId, options)
	if err != nil {
		log.Errorf("Rmove container %s error: %s", containerId, err.Error())
		return err
	}
	return nil
}

func (d *dockerImage) RemoveContainers(containerIds []string) error {
	options := types.ContainerRemoveOptions{Force: true}
	for _, v := range containerIds {
		err := d.client.ContainerRemove(context.Background(), v, options)
		if err != nil {
			log.Errorf("Rmove container %s error: %s", v, err.Error())
			return err
		}
	}
	return nil
}

func (d *dockerImage) CheckImagesExistInDockerRegistry(name imageName) (bool, error) {
	if name.DockerRegistry == "" {
		return false, errors.New("Docker registry cannot empty")
	}

	var url = ""
	if strings.HasSuffix(name.DockerRegistry, "/") {
		url = "http://" + name.DockerRegistry + "2/" + GetImageName(name.Name) + "/tags/list"
	} else {
		url = "http://" + name.DockerRegistry + "/2/" + GetImageName(name.Name) + "/tags/list"
	}

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return false, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, err
	}

	results := map[string]interface{}{}

	err = json.Unmarshal(body, &results)
	if err != nil {
		return false, err
	}

	tags := results["tags"].([]string)

	for _, tag := range tags {

		if strings.EqualFold(tag, GetImageTag(name.Name)) {
			return true, nil
		}
	}

	return false, nil
}


func GetImageName(imageName string) string {
	if strings.Index(imageName, ":") > 0 {

		return strings.Split(imageName, ":")[0]

	}
	return imageName
}

func GetImageTag(imageName string) string {
	if strings.Index(imageName, ":") > 0 {

		return strings.Split(imageName, ":")[1]

	}
	return imageName
}