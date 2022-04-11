package eventhandler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"keptn-contrib/job-executor-service/pkg/config"
	"keptn-contrib/job-executor-service/pkg/k8sutils"
	k8sutilsfake "keptn-contrib/job-executor-service/pkg/k8sutils/fake"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/golang/mock/gomock"
	"github.com/keptn/go-utils/pkg/lib/keptn"
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	keptnfake "github.com/keptn/go-utils/pkg/lib/v0_2_0/fake"
)

const testEvent = `
{
      "project": "sockshop",
      "stage": "dev",
      "service": "carts",
      "labels": {
        "testId": "4711",
        "buildId": "build-17",
        "owner": "JohnDoe"
      },
      "status": "succeeded",
      "result": "pass",
      "action": {
        "name": "run locust tests",
        "action": "hello",
        "description": "so something as defined in remediation.yaml",
        "value" : "1"
      }
}`

const jobName1 = "job-executor-service-job-f2b878d3-03c0-4e8f-bc3f-454b-1"
const jobName2 = "job-executor-service-job-f2b878d3-03c0-4e8f-bc3f-454b-2"

type acceptAllImagesFilter struct {
	ImageFilter
}

func (f acceptAllImagesFilter) IsImageAllowed(_ string) bool {
	return true
}

func createK8sMock(t *testing.T) *k8sutilsfake.MockK8s {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	return k8sutilsfake.NewMockK8s(mockCtrl)
}

/**
 * loads a cloud event from the passed test json file and initializes a keptn object with it
 */
func initializeTestObjects(eventFileName string) (*keptnv2.Keptn, *cloudevents.Event, *keptnfake.EventSender, error) {
	// load sample event
	eventFile, err := ioutil.ReadFile(eventFileName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Cant load %s: %s", eventFileName, err.Error())
	}

	incomingEvent := &cloudevents.Event{}

	err = json.Unmarshal(eventFile, incomingEvent)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error parsing: %s", err.Error())
	}

	// Add a Fake EventSender to KeptnOptions
	fakeEventSender := &keptnfake.EventSender{}
	var keptnOptions = keptn.KeptnOpts{
		EventSender: fakeEventSender,
	}
	keptnOptions.UseLocalFileSystem = true
	myKeptn, err := keptnv2.NewKeptn(incomingEvent, keptnOptions)

	return myKeptn, incomingEvent, fakeEventSender, err
}

func TestStartK8s(t *testing.T) {
	jobNamespace1 := "keptn"
	jobNamespace2 := "keptn-2"
	myKeptn, _, fakeEventSender, err := initializeTestObjects("../../test-events/action.triggered.json")
	require.NoError(t, err)

	eventData := &keptnv2.EventData{}
	myKeptn.CloudEvent.DataAs(eventData)
	eh := EventHandler{
		ServiceName: "job-executor-service",
		Keptn:       myKeptn,
		ImageFilter: acceptAllImagesFilter{},
		JobSettings: k8sutils.JobSettings{
			JobNamespace: jobNamespace1,
		},
	}
	mapper := new(KeptnCloudEventMapper)
	eventPayloadAsInterface, err := mapper.Map(*eh.Keptn.CloudEvent)

	maxPollDuration := 1006
	action := config.Action{
		Name: "Run locust",
		Tasks: []config.Task{
			{
				Name:            "Run locust smoked ham tests",
				MaxPollDuration: &maxPollDuration,
			},
			{
				Name:      "Run locust healthy snack tests",
				Namespace: jobNamespace2,
			},
		},
	}

	k8sMock := createK8sMock(t)
	k8sMock.EXPECT().ConnectToCluster().Times(1)
	k8sMock.EXPECT().CreateK8sJob(
		gomock.Eq(jobName1), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), jobNamespace1,
	).Times(1)
	k8sMock.EXPECT().CreateK8sJob(
		gomock.Eq(jobName2), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), jobNamespace2,
	).Times(1)
	k8sMock.EXPECT().AwaitK8sJobDone(gomock.Eq(jobName1), 202, 5, jobNamespace1).Times(1)
	k8sMock.EXPECT().AwaitK8sJobDone(gomock.Eq(jobName2), 60, 5, jobNamespace2).Times(1)
	k8sMock.EXPECT().GetLogsOfPod(gomock.Eq(jobName1), jobNamespace1).Times(1)
	k8sMock.EXPECT().GetLogsOfPod(gomock.Eq(jobName2), jobNamespace2).Times(1)

	eh.startK8sJob(k8sMock, &action, eventPayloadAsInterface)

	err = fakeEventSender.AssertSentEventTypes(
		[]string{
			"sh.keptn.event.action.started", "sh.keptn.event.action.finished",
		},
	)
	assert.NoError(t, err)
}

func TestStartK8sJobSilent(t *testing.T) {
	myKeptn, _, fakeEventSender, err := initializeTestObjects("../../test-events/action.triggered.json")
	require.NoError(t, err)

	eventData := &keptnv2.EventData{}
	myKeptn.CloudEvent.DataAs(eventData)
	eh := EventHandler{
		ServiceName: "job-executor-service",
		Keptn:       myKeptn,
		ImageFilter: acceptAllImagesFilter{},
	}
	mapper := new(KeptnCloudEventMapper)
	eventPayloadAsInterface, err := mapper.Map(*eh.Keptn.CloudEvent)

	action := config.Action{
		Name: "Run locust",
		Tasks: []config.Task{
			{
				Name: "Run locust smoked ham tests",
			},
			{
				Name: "Run locust healthy snack tests",
			},
		},
		Silent: true,
	}

	k8sMock := createK8sMock(t)
	k8sMock.EXPECT().ConnectToCluster().Times(1)
	k8sMock.EXPECT().CreateK8sJob(
		gomock.Eq(jobName1), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(),
	).Times(1)
	k8sMock.EXPECT().CreateK8sJob(
		gomock.Eq(jobName2), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(),
	).Times(1)
	k8sMock.EXPECT().AwaitK8sJobDone(gomock.Any(), 60, 5, "").Times(2)
	k8sMock.EXPECT().GetLogsOfPod(gomock.Eq(jobName1), gomock.Any()).Times(1)
	k8sMock.EXPECT().GetLogsOfPod(gomock.Eq(jobName2), gomock.Any()).Times(1)

	eh.startK8sJob(k8sMock, &action, eventPayloadAsInterface)

	err = fakeEventSender.AssertSentEventTypes([]string{})
	assert.NoError(t, err)
}

func TestStartK8s_TestFinishedEvent(t *testing.T) {
	myKeptn, _, fakeEventSender, err := initializeTestObjects("../../test-events/test.triggered.json")
	require.NoError(t, err)

	eventData := &keptnv2.EventData{}
	myKeptn.CloudEvent.DataAs(eventData)
	eh := EventHandler{
		ServiceName: "job-executor-service",
		Keptn:       myKeptn,
		ImageFilter: acceptAllImagesFilter{},
	}
	mapper := new(KeptnCloudEventMapper)
	eventPayloadAsInterface, err := mapper.Map(*eh.Keptn.CloudEvent)

	action := config.Action{
		Name: "Run locust",
		Tasks: []config.Task{
			{
				Name: "Run locust healthy snack tests",
			},
		},
	}

	k8sMock := createK8sMock(t)
	k8sMock.EXPECT().ConnectToCluster().Times(1)
	k8sMock.EXPECT().CreateK8sJob(
		gomock.Eq(jobName1), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(),
	).Times(1)
	k8sMock.EXPECT().AwaitK8sJobDone(gomock.Eq(jobName1), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	k8sMock.EXPECT().GetLogsOfPod(gomock.Eq(jobName1), gomock.Any()).Times(1)

	// set the global timezone for testing
	local, err := time.LoadLocation("UTC")
	require.NoError(t, err)
	time.Local = local

	eh.startK8sJob(k8sMock, &action, eventPayloadAsInterface)

	err = fakeEventSender.AssertSentEventTypes(
		[]string{
			keptnv2.GetStartedEventType(keptnv2.TestTaskName),
			keptnv2.GetFinishedEventType(keptnv2.TestTaskName),
		},
	)
	require.NoError(t, err)

	for _, cloudEvent := range fakeEventSender.SentEvents {
		if cloudEvent.Type() == keptnv2.GetFinishedEventType(keptnv2.TestTaskName) {
			eventData := &keptnv2.TestFinishedEventData{}
			cloudEvent.DataAs(eventData)

			dateLayout := "2006-01-02T15:04:05Z"
			_, err := time.Parse(dateLayout, eventData.Test.Start)
			assert.NoError(t, err)
			_, err = time.Parse(dateLayout, eventData.Test.End)
			assert.NoError(t, err)
		}
	}
}

type disallowAllImagesFilter struct {
	ImageFilter
}

func (f disallowAllImagesFilter) IsImageAllowed(_ string) bool {
	return false
}

func TestExpectImageNotAllowedError(t *testing.T) {
	myKeptn, _, fakeEventSender, err := initializeTestObjects("../../test-events/test.triggered.json")
	require.NoError(t, err)

	eventData := &keptnv2.EventData{}
	myKeptn.CloudEvent.DataAs(eventData)
	eh := EventHandler{
		ServiceName: "job-executor-service",
		Keptn:       myKeptn,
		ImageFilter: disallowAllImagesFilter{},
	}
	mapper := new(KeptnCloudEventMapper)
	eventPayloadAsInterface, err := mapper.Map(*eh.Keptn.CloudEvent)

	notAllowedImageName := "alpine:latest"
	action := config.Action{
		Name: "Run some task with invalid image",
		Tasks: []config.Task{
			{
				Image: notAllowedImageName,
				Name:  "Run some image",
			},
		},
	}

	k8sMock := createK8sMock(t)
	k8sMock.EXPECT().ConnectToCluster().Times(1)
	k8sMock.EXPECT().CreateK8sJob(
		gomock.Eq(jobName1), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(),
	).Times(1)
	k8sMock.EXPECT().AwaitK8sJobDone(gomock.Eq(jobName1), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
	k8sMock.EXPECT().GetLogsOfPod(gomock.Eq(jobName1), gomock.Any()).Times(1)

	// set the global timezone for testing
	local, err := time.LoadLocation("UTC")
	require.NoError(t, err)
	time.Local = local

	eh.startK8sJob(k8sMock, &action, eventPayloadAsInterface)

	err = fakeEventSender.AssertSentEventTypes(
		[]string{
			keptnv2.GetStartedEventType(keptnv2.TestTaskName),
			keptnv2.GetFinishedEventType(keptnv2.TestTaskName),
		},
	)
	require.NoError(t, err)

	for _, cloudEvent := range fakeEventSender.SentEvents {
		if cloudEvent.Type() == keptnv2.GetFinishedEventType(keptnv2.TestTaskName) {
			eventData := &keptnv2.TestFinishedEventData{}
			cloudEvent.DataAs(eventData)

			assert.Equal(t, eventData.Status, keptnv2.StatusErrored)
			assert.Equal(t, eventData.Result, keptnv2.ResultFailed)
			assert.Contains(t, eventData.Message, notAllowedImageName)
		}
	}
}
