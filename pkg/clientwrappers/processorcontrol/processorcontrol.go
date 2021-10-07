package processorcontrol

import (
	//"strings"

	"github.com/Orange-OpenSource/nifikop/api/v1alpha1"
	"github.com/Orange-OpenSource/nifikop/pkg/clientwrappers"
	"github.com/Orange-OpenSource/nifikop/pkg/common"
	//"github.com/Orange-OpenSource/nifikop/pkg/errorfactory"
	"github.com/Orange-OpenSource/nifikop/pkg/nificlient"
	nigoapi "github.com/erdrix/nigoapi/pkg/nifi"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = ctrl.Log.WithName("processorcontrol-method")

// ProcessorExist check if the Processor exist on NiFi Cluster
func ProcessorExist(client client.Client, processor *v1alpha1.ProcessorControl, cluster *v1alpha1.NifiCluster) (bool, error) {

	if processor.Status.ProcessorID == "" {
		return false, nil
	}

	nClient, err := common.NewNodeConnection(log, client, cluster)
	if err != nil {
		return false, err
	}

	processorEntity, err := nClient.GetProcessor(processor.Status.ProcessorID)
	if err := clientwrappers.ErrorGetOperation(log, err, "Get processor"); err != nil {
		if err == nificlient.ErrNifiClusterReturned404 {
			return false, nil
		}
		return false, err
	}

	return processorEntity != nil, nil
}

// ScheduleProcessor will schedule the processor in the sequence listed in ProcessorControl.
func ScheduleProcessor(client client.Client, processor *v1alpha1.ProcessorControl, cluster *v1alpha1.NifiCluster) error {
	nClient, err := common.NewNodeConnection(log, client, cluster)
	if err != nil {
		return err
	}

	// Schedule processor
	_, err = nClient.UpdateProcessorRunStatus(processor.Status.ProcessorID, nigoapi.ProcessorRunStatusEntity{
		State: "RUNNING",
	})
	if err := clientwrappers.ErrorUpdateOperation(log, err, "Schedule processors"); err != nil {
		return err
	}

	return nil
}