package agentgatewaysyncer

import (
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/workloadapi"
)

func PrecomputeWorkload(w model.WorkloadInfo) model.WorkloadInfo {
	addr := workloadToAddress(w.Workload)
	w.MarshaledAddress = protoconv.MessageToAny(addr)
	w.AsAddress = model.AddressInfo{
		Address:   addr,
		Marshaled: w.MarshaledAddress,
	}
	return w
}

func workloadToAddress(w *workloadapi.Workload) *workloadapi.Address {
	return &workloadapi.Address{
		Type: &workloadapi.Address_Workload{
			Workload: w,
		},
	}
}
