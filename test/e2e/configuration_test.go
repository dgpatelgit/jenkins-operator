package e2e

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jenkinsci/kubernetes-operator/pkg/apis/jenkins/v1alpha2"
	"github.com/jenkinsci/kubernetes-operator/pkg/apis/jenkins/v1alpha3"
	jenkinsclient "github.com/jenkinsci/kubernetes-operator/pkg/client"
	"github.com/jenkinsci/kubernetes-operator/pkg/configuration/base"
	"github.com/jenkinsci/kubernetes-operator/pkg/configuration/base/resources"
	"github.com/jenkinsci/kubernetes-operator/pkg/plugins"

	"github.com/bndr/gojenkins"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const e2e = "e2e"
const cascE2e = "casc-e2e"

func TestConfiguration(t *testing.T) {
	t.Parallel()
	namespace, ctx := setupTest(t)

	defer showLogsAndCleanup(t, ctx)

	jenkinsCRName := e2e
	cascCRName := cascE2e
	numberOfExecutors := 6
	numberOfExecutorsEnvName := "NUMBER_OF_EXECUTORS"
	systemMessage := "Configuration as Code integration works!!!"
	systemMessageEnvName := "SYSTEM_MESSAGE"
	priorityClassName := ""
	mySeedJob := seedJobConfig{
		SeedJob: v1alpha2.SeedJob{
			ID:                    "jenkins-operator",
			CredentialID:          "jenkins-operator",
			JenkinsCredentialType: v1alpha2.NoJenkinsCredentialCredentialType,
			Targets:               "cicd/jobs/*.jenkins",
			Description:           "Jenkins Operator repository",
			RepositoryBranch:      "master",
			RepositoryURL:         "https://github.com/jenkinsci/kubernetes-operator.git",
			PollSCM:               "1 1 1 1 1",
			UnstableOnDeprecation: true,
			BuildPeriodically:     "1 1 1 1 1",
			FailOnMissingPlugin:   true,
			IgnoreMissingFiles:    true,
			//AdditionalClasspath: can fail with the seed job agent
			GitHubPushTrigger: true,
		},
	}
	groovyScripts := v1alpha3.Customization{
		Configurations: []v1alpha3.ConfigMapRef{
			{
				Name: userConfigurationConfigMapName,
			},
		},
		Secret: v1alpha3.SecretRef{
			Name: userConfigurationSecretName,
		},
	}

	cascConfig := v1alpha3.Customization{
		Configurations: []v1alpha3.ConfigMapRef{
			{
				Name: userConfigurationConfigMapName,
			},
		},
		Secret: v1alpha3.SecretRef{
			Name: userConfigurationSecretName,
		},
	}

	stringData := make(map[string]string)
	stringData[systemMessageEnvName] = systemMessage
	stringData[numberOfExecutorsEnvName] = strconv.Itoa(numberOfExecutors)

	// base
	createUserConfigurationSecret(t, namespace, stringData)
	createUserConfigurationConfigMap(t, namespace, numberOfExecutors, systemMessageEnvName)
	jenkins := createJenkinsCR(t, jenkinsCRName, namespace, priorityClassName, &[]v1alpha2.SeedJob{mySeedJob.SeedJob})
	//jenkins.Annotations[base.UseDeploymentAnnotation] = "false"
	createDefaultLimitsForContainersInNamespace(t, namespace)
	createKubernetesCredentialsProviderSecret(t, namespace, mySeedJob)
	waitForJenkinsBaseConfigurationToComplete(t, jenkins)
	verifyJenkinsMasterPodAttributes(t, jenkins)
	jenkinsClient, cleanUpFunc := verifyJenkinsAPIConnection(t, jenkins, namespace)
	defer cleanUpFunc()
	verifyPlugins(t, jenkinsClient, jenkins)

	// user
	casc := createCascCR(t, jenkinsCRName, cascCRName, namespace, groovyScripts, cascConfig)
	waitForJenkinsUserConfigurationToComplete(t, casc)
	verifyUserConfiguration(t, jenkinsClient, 6, systemMessage)
	time.Sleep(60 * time.Second)
	verifyJenkinsSeedJobs(t, jenkinsClient, []seedJobConfig{mySeedJob})
}

func TestPlugins(t *testing.T) {
	t.Parallel()
	namespace, ctx := setupTest(t)
	// Deletes test namespace
	defer showLogsAndCleanup(t, ctx)

	jobID := "k8s-e2e"

	priorityClassName := ""
	seedJobs := &[]v1alpha2.SeedJob{
		{
			ID:                    "jenkins-operator",
			CredentialID:          "jenkins-operator",
			JenkinsCredentialType: v1alpha2.NoJenkinsCredentialCredentialType,
			Targets:               "cicd/jobs/k8s.jenkins",
			Description:           "Jenkins Operator repository",
			RepositoryBranch:      "master",
			RepositoryURL:         "https://github.com/jenkinsci/kubernetes-operator.git",
		},
	}

	jenkins := createJenkinsCR(t, "k8s-e2e", namespace, priorityClassName, seedJobs)
	waitForJenkinsBaseConfigurationToComplete(t, jenkins)

	jenkinsClient, cleanUpFunc := verifyJenkinsAPIConnection(t, jenkins, namespace)
	defer cleanUpFunc()
	waitForJob(t, jenkinsClient, jobID)
	job, err := jenkinsClient.GetJob(jobID)

	require.NoError(t, err, job)
	i, err := job.InvokeSimple(map[string]string{})
	require.NoError(t, err, i)

	waitForJobToFinish(t, job, 2*time.Second, 2*time.Minute)

	job, err = jenkinsClient.GetJob(jobID)
	require.NoError(t, err, job)
	build, err := job.GetLastBuild()
	require.NoError(t, err)
	assert.True(t, build.IsGood())
}

func TestPriorityClass(t *testing.T) {
	if skipTestPriorityClass {
		t.Skip()
	}
	t.Parallel()
	namespace, ctx := setupTest(t)
	defer showLogsAndCleanup(t, ctx)

	jenkinsCRName := "k8s-ete-priority-class-existing"
	// One of the existing priority classes
	priorityClassName := "system-cluster-critical"

	jenkins := createJenkinsCR(t, jenkinsCRName, namespace, priorityClassName, nil)
	waitForJenkinsBaseConfigurationToComplete(t, jenkins)
	verifyJenkinsMasterPodAttributes(t, jenkins)
	_, cleanUpFunc := verifyJenkinsAPIConnection(t, jenkins, namespace)
	defer cleanUpFunc()
}

func createUserConfigurationSecret(t *testing.T, namespace string, stringData map[string]string) {
	userConfiguration := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userConfigurationSecretName,
			Namespace: namespace,
		},
		StringData: stringData,
	}

	t.Logf("User configuration secret %+v", *userConfiguration)
	if err := framework.Global.Client.Create(context.TODO(), userConfiguration, nil); err != nil {
		t.Fatal(err)
	}
}

func createUserConfigurationConfigMap(t *testing.T, namespace string, numberOfExecutors int, systemMessageEnvName string) {
	userConfiguration := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userConfigurationConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"1-set-executors.groovy": fmt.Sprintf(`
import jenkins.model.Jenkins
Jenkins.instance.setNumExecutors(%d)
Jenkins.instance.save()`, numberOfExecutors),
			"1-casc.yaml": fmt.Sprintf(`
jenkins:
  systemMessage: "${%s}"`, systemMessageEnvName),
			"2-casc.yaml": `
unclassified:
  location:
    url: http://external-jenkins-url:8080`,
		},
	}

	t.Logf("User configuration %+v", *userConfiguration)
	if err := framework.Global.Client.Create(context.TODO(), userConfiguration, nil); err != nil {
		t.Fatal(err)
	}
}

func createDefaultLimitsForContainersInNamespace(t *testing.T, namespace string) {
	limitRange := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      e2e,
			Namespace: namespace,
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					DefaultRequest: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("128m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Default: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceCPU:    resource.MustParse("256m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		},
	}

	t.Logf("LimitRange %+v", *limitRange)
	if err := framework.Global.Client.Create(context.TODO(), limitRange, nil); err != nil {
		t.Fatal(err)
	}
}

func verifyJenkinsMasterPodAttributes(t *testing.T, jenkins *v1alpha2.Jenkins) {
	jenkinsPod := getJenkinsMasterPod(t, jenkins)
	jenkins = getJenkins(t, jenkins.Namespace, jenkins.Name)

	assertMapContainsElementsFromAnotherMap(t, jenkins.Spec.Master.Annotations, jenkinsPod.ObjectMeta.Annotations)
	assert.Equal(t, jenkins.Spec.Master.NodeSelector, jenkinsPod.Spec.NodeSelector)

	assert.Equal(t, resources.JenkinsMasterContainerName, jenkinsPod.Spec.Containers[0].Name)
	assert.Equal(t, len(jenkins.Spec.Master.Containers), len(jenkinsPod.Spec.Containers))

	expectedSecurityContext := jenkins.Spec.Master.SecurityContext
	if expectedSecurityContext == nil {
		expectedSecurityContext = getEmptySecurityContext()
	}
	assert.Equal(t, expectedSecurityContext, jenkinsPod.Spec.SecurityContext)
	assert.Equal(t, jenkins.Spec.Master.Containers[0].Command, jenkinsPod.Spec.Containers[0].Command)

	// The deployment adds "pod-template-hash" as actualLabels; removing it for the comparaison
	actualLabels := jenkinsPod.Labels
	_, ok := actualLabels["pod-template-hash"]
	if ok {
		delete(actualLabels, "pod-template-hash")
	}
	assert.Equal(t, resources.GetJenkinsMasterPodLabels(jenkins), jenkinsPod.Labels)
	assert.Equal(t, jenkins.Spec.Master.PriorityClassName, jenkinsPod.Spec.PriorityClassName)

	for _, actualContainer := range jenkinsPod.Spec.Containers {
		if actualContainer.Name == resources.JenkinsMasterContainerName {
			verifyContainer(t, resources.NewJenkinsMasterContainer(jenkins), actualContainer)
			continue
		}

		var expectedContainer *corev1.Container
		for _, jenkinsContainer := range jenkins.Spec.Master.Containers {
			if jenkinsContainer.Name == actualContainer.Name {
				tmp := resources.ConvertJenkinsContainerToKubernetesContainer(jenkinsContainer)
				expectedContainer = &tmp
			}
		}

		if expectedContainer == nil {
			t.Errorf("Container '%+v' not found in pod", actualContainer)
			continue
		}

		verifyContainer(t, *expectedContainer, actualContainer)
	}

	for _, expectedVolume := range jenkins.Spec.Master.Volumes {
		volumeFound := false
		for _, actualVolume := range jenkinsPod.Spec.Volumes {
			if expectedVolume.Name == actualVolume.Name {
				volumeFound = true
				assert.Equal(t, expectedVolume, actualVolume)
			}
		}

		if !volumeFound {
			t.Errorf("Missing volume '+%v', actaul volumes '%+v'", expectedVolume, jenkinsPod.Spec.Volumes)
		}
	}

	t.Log("Jenkins pod attributes are valid")
}

func getEmptySecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		SELinuxOptions:     nil,
		RunAsUser:          nil,
		RunAsNonRoot:       nil,
		SupplementalGroups: nil,
		FSGroup:            nil,
		RunAsGroup:         nil,
		Sysctls:            nil,
		WindowsOptions:     nil,
	}
}

func verifyContainer(t *testing.T, expected corev1.Container, actual corev1.Container) {
	assert.Equal(t, expected.Args, actual.Args, expected.Name, expected.Name)
	assert.Equal(t, expected.Command, actual.Command, expected.Name)
	assert.ElementsMatch(t, expected.Env, actual.Env, expected.Name)
	assert.Equal(t, expected.EnvFrom, actual.EnvFrom, expected.Name)
	assert.Equal(t, expected.Image, actual.Image, expected.Name)
	assert.Equal(t, expected.ImagePullPolicy, actual.ImagePullPolicy, expected.Name)
	assert.Equal(t, expected.Lifecycle, actual.Lifecycle, expected.Name)
	assert.Equal(t, expected.LivenessProbe, actual.LivenessProbe, expected.Name)
	assert.Equal(t, expected.Ports, actual.Ports, expected.Name)
	assert.Equal(t, expected.ReadinessProbe, actual.ReadinessProbe, expected.Name)
	assert.Equal(t, expected.Resources, actual.Resources, expected.Name)
	assert.Equal(t, expected.SecurityContext, actual.SecurityContext, expected.Name)
	assert.Equal(t, expected.WorkingDir, actual.WorkingDir, expected.Name)
	if !base.CompareContainerVolumeMounts(expected, actual) {
		t.Errorf("Volume mounts are different in container '%s': expected '%+v', actual '%+v'",
			expected.Name, expected.VolumeMounts, actual.VolumeMounts)
	}
}

func verifyPlugins(t *testing.T, jenkinsClient jenkinsclient.Jenkins, jenkins *v1alpha2.Jenkins) {
	installedPlugins, err := jenkinsClient.GetPlugins(1)
	if err != nil {
		t.Fatal(err)
	}

	for _, basePlugin := range plugins.BasePlugins() {
		if found, ok := isPluginValid(installedPlugins, basePlugin); !ok {
			t.Fatalf("Invalid plugin '%s', actual '%+v'", basePlugin, found)
		}
	}

	for _, userPlugin := range jenkins.Spec.Master.Plugins {
		plugin := plugins.Plugin{Name: userPlugin.Name, Version: userPlugin.Version}
		if found, ok := isPluginValid(installedPlugins, plugin); !ok {
			t.Fatalf("Invalid plugin '%s', actual '%+v'", plugin, found)
		}
	}

	t.Log("All plugins have been installed")
}

func isPluginValid(plugins *gojenkins.Plugins, requiredPlugin plugins.Plugin) (*gojenkins.Plugin, bool) {
	p := plugins.Contains(requiredPlugin.Name)
	if p == nil {
		return p, false
	}

	if !p.Active || !p.Enabled || p.Deleted {
		return p, false
	}

	return p, requiredPlugin.Version == p.Version
}

func verifyUserConfiguration(t *testing.T, jenkinsClient jenkinsclient.Jenkins, amountOfExecutors int, systemMessage string) {
	checkConfigurationViaGroovyScript := fmt.Sprintf(`
if (!new Integer(%d).equals(Jenkins.instance.numExecutors)) {
	throw new Exception("Configuration via groovy scripts failed")
}`, amountOfExecutors)
	logs, err := jenkinsClient.ExecuteScript(checkConfigurationViaGroovyScript)
	assert.NoError(t, err, logs)

	checkConfigurationAsCode := fmt.Sprintf(`
if (!"%s".equals(Jenkins.instance.systemMessage)) {
	throw new Exception("Configuration as code failed")
}`, systemMessage)
	logs, err = jenkinsClient.ExecuteScript(checkConfigurationAsCode)
	assert.NoError(t, err, logs)
}

func assertMapContainsElementsFromAnotherMap(t *testing.T, expected map[string]string, actual map[string]string) {
	for key, expectedValue := range expected {
		actualValue, keyExists := actual[key]
		if !keyExists {
			assert.Failf(t, "key '%s' doesn't exist in map '%+v'", key, actual)
			continue
		}
		assert.Equal(t, expectedValue, actualValue, expected, actual)
	}
}
