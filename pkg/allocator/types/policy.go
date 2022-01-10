package types

type NodeList []string

// Policy interface defines two functions to handle single job allocation and global jobs optimization.
type Policy interface {
	Allocate(job JobInfo, nodes map[string]NodeInfo) (NodeList, error)
	Optimize(jobs map[string]JobInfo, nodes map[string]NodeInfo, prevAllocations map[string]NodeList) (map[string]NodeList, error)
}
