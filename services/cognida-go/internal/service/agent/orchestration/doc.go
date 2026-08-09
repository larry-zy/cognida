// Package orchestration provides composable patterns for combining multiple agents.
//
// All orchestration patterns implement the Agent interface, allowing them to be
// used anywhere an Agent is expected and nested within other orchestrations.
//
// # Patterns
//
//   - Sequential - Execute agents in order, passing output to the next
//   - Parallel - Execute agents concurrently with the same input
//   - Supervisor - Route requests to worker agents based on coordinator decision
//   - Conditional/Branch - Select agent based on predicate functions
//   - Loop/Repeat/Retry - Execute agents repeatedly with control flow
//   - Func - Create an agent from a simple function
//
// # Example
//
//	// Create a pipeline
//	pipeline := orchestration.Sequential(
//	    agent1,
//	    agent2,
//	    agent3,
//	)
//
//	// Parallel execution
//	parallel := orchestration.Parallel(
//	    agent1,
//	    agent2,
//	)
//
//	// Combine patterns
//	complex := orchestration.Sequential(
//	    orchestration.Parallel(agent1, agent2),
//	    agent3,
//	)
package orchestration
