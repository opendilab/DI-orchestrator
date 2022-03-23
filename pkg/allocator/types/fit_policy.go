package types

// FitPolicy is an implementation of Policy interface.
type FitPolicy struct{}

func NewFitPolicy() *FitPolicy {
	return &FitPolicy{}
}

func (p FitPolicy) Allocate(job JobInfo, nodes map[string]*NodeInfo) (NodeList, error) {
	return NodeList{}, nil
}

func (p FitPolicy) Optimize(jobs map[string]JobInfo, nodes map[string]*NodeInfo, prevAllocations map[string]NodeList) (map[string]NodeList, error) {
	return map[string]NodeList{}, nil
}
