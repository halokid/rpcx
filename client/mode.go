package client

//FailMode decides how clients action when clients fail to invoke services
type FailMode int

const (
	// todo: iota为优雅的定义golang的常量， Failover为1， 其他常量依次递增， 比如Failfast为2
	//Failover selects another server automaticaly
	Failover FailMode = iota
	//Failfast returns error immediately
	Failfast
	//Failtry use current client again
	Failtry
	//Failbackup select another server if the first server doesn't respon in specified time and use the fast response.
	Failbackup
)

// SelectMode defines the algorithm of selecting a services from candidates.
type SelectMode int

const (
	//RandomSelect is selecting randomly
	RandomSelect SelectMode = iota
	//RoundRobin is selecting by round robin
	RoundRobin
	//WeightedRoundRobin is selecting by weighted round robin
	WeightedRoundRobin
	//WeightedICMP is selecting by weighted Ping time
	WeightedICMP
	//ConsistentHash is selecting by hashing
	ConsistentHash
	//Closest is selecting the closest server
	Closest

	// SelectByUser is selecting by implementation of users
	SelectByUser = 1000
)
