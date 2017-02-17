/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package policies

import (
	"fmt"
	"strings"

	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
)

const (
	// ChannelReaders is the label for the channel's readers policy
	ChannelReaders = "ChannelReaders"
)

var logger = logging.MustGetLogger("common/policies")

// Policy is used to determine if a signature is valid
type Policy interface {
	// Evaluate takes a set of SignedData and evaluates whether this set of signatures satisfies the policy
	Evaluate(signatureSet []*cb.SignedData) error
}

// Manager is intended to be the primary accessor of ManagerImpl
// It is intended to discourage use of the other exported ManagerImpl methods
// which are used for updating policy by the ConfigManager
type Manager interface {
	// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default policy
	GetPolicy(id string) (Policy, bool)
}

// Provider provides the backing implementation of a policy
type Provider interface {
	// NewPolicy creates a new policy based on the policy bytes
	NewPolicy(data []byte) (Policy, error)
}

// ManagerImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.ConfigManager
type ManagerImpl struct {
	providers       map[int32]Provider
	policies        map[string]Policy
	pendingPolicies map[string]Policy
}

// NewManagerImpl creates a new ManagerImpl with the given CryptoHelper
func NewManagerImpl(providers map[int32]Provider) *ManagerImpl {
	return &ManagerImpl{
		providers: providers,
		policies:  make(map[string]Policy),
	}
}

type rejectPolicy string

func (rp rejectPolicy) Evaluate(signedData []*cb.SignedData) error {
	return fmt.Errorf("No such policy type: %s", rp)
}

// GetPolicy returns a policy and true if it was the policy requested, or false if it is the default reject policy
func (pm *ManagerImpl) GetPolicy(id string) (Policy, bool) {
	policy, ok := pm.policies[id]
	if !ok {
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Returning dummy reject all policy because %s could not be found", id)
		}
		return rejectPolicy(id), false
	}
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Returning policy %s for evaluation", id)
	}
	return policy, true
}

// BeginConfig is used to start a new config proposal
func (pm *ManagerImpl) BeginConfig(groups []string) ([]*ManagerImpl, error) {
	if len(groups) != 0 {
		return nil, fmt.Errorf("Unimplemented") // XXX Temporary workaround until policy manager is enhanced to support hierarchy
	}
	if pm.pendingPolicies != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	pm.pendingPolicies = make(map[string]Policy)
	return nil, nil
}

// RollbackConfig is used to abandon a new config proposal
func (pm *ManagerImpl) RollbackConfig() {
	pm.pendingPolicies = nil
}

// CommitConfig is used to commit a new config proposal
func (pm *ManagerImpl) CommitConfig() {
	if pm.pendingPolicies == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	pm.policies = pm.pendingPolicies
	pm.pendingPolicies = nil
}

// ProposePolicy takes key, path, and ConfigPolicy and registers it in the proposed PolicyManager, or errors
func (pm *ManagerImpl) ProposePolicy(key string, path []string, configPolicy *cb.ConfigPolicy) error {
	policy := configPolicy.Policy
	if policy == nil {
		return fmt.Errorf("Policy cannot be nil")
	}

	provider, ok := pm.providers[int32(policy.Type)]
	if !ok {
		return fmt.Errorf("Unknown policy type: %v", policy.Type)
	}

	cPolicy, err := provider.NewPolicy(policy.Policy)
	if err != nil {
		return err
	}

	prefix := strings.Join(path, "/")
	if len(path) == 0 {
		prefix = "/"
	}

	// TODO, once the other components are ready for it, use '_' below as fqKey
	_ = prefix + "/" + key
	fqKey := key

	logger.Debugf("Writing policy with fqKey: %s", fqKey)

	pm.pendingPolicies[fqKey] = cPolicy

	logger.Debugf("Proposed new policy %s", key)
	return nil
}
