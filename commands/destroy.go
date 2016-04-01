package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/pivotal-cf-experimental/bosh-bootloader/aws"
	"github.com/pivotal-cf-experimental/bosh-bootloader/aws/cloudformation"
	"github.com/pivotal-cf-experimental/bosh-bootloader/boshinit"
	"github.com/pivotal-cf-experimental/bosh-bootloader/storage"
)

type Destroy struct {
	logger                logger
	stdin                 io.Reader
	boshDeleter           boshDeleter
	clientProvider        clientProvider
	stackManager          stackManager
	stringGenerator       stringGenerator
	infrastructureManager infrastructureManager
}

type boshDeleter interface {
	Delete(boshinit.DeployInput) error
}

type clientProvider interface {
	CloudFormationClient(aws.Config) (cloudformation.Client, error)
}

type stackManager interface {
	Describe(cloudformation.Client, string) (cloudformation.Stack, error)
}

type stringGenerator interface {
	Generate(prefix string, length int) (string, error)
}

func NewDestroy(logger logger, stdin io.Reader, boshDeleter boshDeleter, clientProvider clientProvider, stackManager stackManager, stringGenerator stringGenerator, infrastructureManager infrastructureManager) Destroy {
	return Destroy{
		logger:                logger,
		stdin:                 stdin,
		boshDeleter:           boshDeleter,
		clientProvider:        clientProvider,
		stackManager:          stackManager,
		stringGenerator:       stringGenerator,
		infrastructureManager: infrastructureManager,
	}
}

func (d Destroy) Execute(globalFlags GlobalFlags, state storage.State) (storage.State, error) {
	d.logger.Prompt("Are you sure you want to delete your infrastructure? This operation cannot be undone!")

	var proceed string
	fmt.Fscanln(d.stdin, &proceed)

	proceed = strings.ToLower(proceed)
	if proceed != "yes" && proceed != "y" {
		d.logger.Step("exiting")
		return state, nil
	}

	d.logger.Step("destroying infrastructure")

	cloudFormationClient, err := d.clientProvider.CloudFormationClient(aws.Config{
		AccessKeyID:      state.AWS.AccessKeyID,
		SecretAccessKey:  state.AWS.SecretAccessKey,
		Region:           state.AWS.Region,
		EndpointOverride: globalFlags.EndpointOverride,
	})
	if err != nil {
		return state, err
	}

	stack, err := d.stackManager.Describe(cloudFormationClient, state.Stack.Name)
	if err != nil {
		return state, err
	}

	infrastructureConfiguration := boshinit.InfrastructureConfiguration{
		AWSRegion:        state.AWS.Region,
		SubnetID:         stack.Outputs["BOSHSubnet"],
		AvailabilityZone: stack.Outputs["BOSHSubnetAZ"],
		ElasticIP:        stack.Outputs["BOSHEIP"],
		AccessKeyID:      stack.Outputs["BOSHUserAccessKey"],
		SecretAccessKey:  stack.Outputs["BOSHUserSecretAccessKey"],
		SecurityGroup:    stack.Outputs["BOSHSecurityGroup"],
	}

	deployInput, err := boshinit.NewDeployInput(state, infrastructureConfiguration, d.stringGenerator)
	if err != nil {
		return state, err
	}

	if err := d.boshDeleter.Delete(deployInput); err != nil {
		return state, err
	}

	if err := d.infrastructureManager.Delete(cloudFormationClient, state.Stack.Name); err != nil {
		return state, err
	}

	return state, nil
}