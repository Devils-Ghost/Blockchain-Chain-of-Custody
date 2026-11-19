package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type ChainOfCustodyContract struct {
	contractapi.Contract
}

type Evidence struct {
	ID          string   `json:"id"`	
	Description string   `json:"description"`
	Owner       string   `json:"owner"`
	Location    string   `json:"location"`
	Status      string   `json:"status"`	
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Tags        []string `json:"tags"`
}

type CustodyTransfer struct {
	EvidenceID   string `json:"evidence_id"`
	FromOwner    string `json:"from_owner"`
	ToOwner      string `json:"to_owner"`
	Timestamp    string `json:"timestamp"`
	Reason       string `json:"reason"`
	TransferredBy string `json:"transferred_by"`
}


func (c *ChainOfCustodyContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
	now, err := txTime(ctx)
	if err != nil {
		return err
	}
	evidences := []Evidence{
		{
			ID:          "EV001",
			Description: "Sample Evidence Item",
			Owner:       "Officer Smith",
			Location:    "Evidence Locker A1",
			Status:      "Collected",
			CreatedAt:   now,
			UpdatedAt:   now,
			Tags:        []string{"sample", "test"},
		},
	}

	for _, evidence := range evidences {
		evidenceJSON, err := json.Marshal(evidence)
		if err != nil {
			return err
		}

		err = ctx.GetStub().PutState(evidence.ID, evidenceJSON)
		if err != nil {
			return fmt.Errorf("failed to put evidence to world state: %v", err)
		}
	}

	return nil
}

func (c *ChainOfCustodyContract) validateNewEvidenceID(ctx contractapi.TransactionContextInterface, id string) error {
    if strings.TrimSpace(id) == "" {
        return fmt.Errorf("evidence id must not be empty")
    }

    exists, err := c.EvidenceExists(ctx, id)
    if err != nil {
        return err
    }
    if exists {
        return fmt.Errorf("evidence %s already exists", id)
    }

    return nil
}

func (c *ChainOfCustodyContract) CreateEvidence(ctx contractapi.TransactionContextInterface, id string, description string, owner string, location string, tags []string) error {
    if err := c.validateNewEvidenceID(ctx, id); err != nil {
        return err
    }

    now, err := txTime(ctx)
    if err != nil {
        return err
    }

    evidence := Evidence{
        ID:          id,
        Description: description,
        Owner:       owner,
        Location:    location,
        Status:      "Collected",
        CreatedAt:   now,
        UpdatedAt:   now,
        Tags:        tags,
    }

    evidenceJSON, err := json.Marshal(evidence)
    if err != nil {
        return err
    }

    err = ctx.GetStub().PutState(id, evidenceJSON)
    if err != nil {
        return err
    }

    return c.addToEvidenceIndex(ctx, id)
}

func (c *ChainOfCustodyContract) ReadEvidence(ctx contractapi.TransactionContextInterface, id string) (*Evidence, error) {
	evidenceJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return nil, fmt.Errorf("failed to read evidence: %v", err)
	}
	if evidenceJSON == nil {
		return nil, fmt.Errorf("evidence %s does not exist", id)
	}

	var evidence Evidence
	err = json.Unmarshal(evidenceJSON, &evidence)
	if err != nil {
		return nil, err
	}

	return &evidence, nil
}

func (c *ChainOfCustodyContract) UpdateEvidence(ctx contractapi.TransactionContextInterface, id string, description string, location string, status string) error {
	evidence, err := c.ReadEvidence(ctx, id)
	if err != nil {
		return err
	}

	evidence.Description = description
	evidence.Location = location
	evidence.Status = status
	evidence.UpdatedAt, err = txTime(ctx)
	if err != nil {
		return err
	}

	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(id, evidenceJSON)
}

// txTime returns the transaction's client-set timestamp in RFC3339. Every endorsing peer
// reads the same value, unlike time.Now(), which differs per peer and can make the
// endorsements of a multi-org transaction disagree.
func txTime(ctx contractapi.TransactionContextInterface) (string, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", fmt.Errorf("failed to read transaction timestamp: %v", err)
	}
	return time.Unix(ts.Seconds, int64(ts.Nanos)).UTC().Format(time.RFC3339), nil
}

// callerIdentity returns the submitting client's verified identity as "<MSP ID>:<certificate CN>".
// Fabric checks the client's signature against its MSP before the chaincode runs, so this
// value cannot be forged by the caller, unlike a name passed in as an argument.
func callerIdentity(ctx contractapi.TransactionContextInterface) (string, error) {
	clientID := ctx.GetClientIdentity()
	mspID, err := clientID.GetMSPID()
	if err != nil {
		return "", fmt.Errorf("failed to read caller MSP ID: %v", err)
	}
	cert, err := clientID.GetX509Certificate()
	if err != nil {
		return "", fmt.Errorf("failed to read caller certificate: %v", err)
	}
	if cert == nil || cert.Subject.CommonName == "" {
		return "", fmt.Errorf("caller certificate has no common name")
	}
	return mspID + ":" + cert.Subject.CommonName, nil
}

func (c *ChainOfCustodyContract) TransferCustody(ctx contractapi.TransactionContextInterface, id string, newOwner string, reason string) error {
	evidence, err := c.ReadEvidence(ctx, id)
	if err != nil {
		return err
	}

	transferredBy, err := callerIdentity(ctx)
	if err != nil {
		return err
	}

	now, err := txTime(ctx)
	if err != nil {
		return err
	}

	transfer := CustodyTransfer{
		EvidenceID:   id,
		FromOwner:    evidence.Owner,
		ToOwner:      newOwner,
		Timestamp:    now,
		Reason:       reason,
		TransferredBy: transferredBy,
	}

	transferJSON, err := json.Marshal(transfer)
	if err != nil {
		return err
	}

	// The transaction ID keeps two transfers of one item in the same second from colliding.
	transferKey := fmt.Sprintf("TRANSFER_%s_%s", id, ctx.GetStub().GetTxID())
	err = ctx.GetStub().PutState(transferKey, transferJSON)
	if err != nil {
		return err
	}

	evidence.Owner = newOwner
	evidence.UpdatedAt = now

	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return err
	}

	return ctx.GetStub().PutState(id, evidenceJSON)
}

func (c *ChainOfCustodyContract) GetEvidenceHistory(ctx contractapi.TransactionContextInterface, id string) ([]map[string]interface{}, error) {
	resultsIterator, err := ctx.GetStub().GetHistoryForKey(id)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var history []map[string]interface{}

	for resultsIterator.HasNext() {
		response, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var evidence Evidence
		if len(response.Value) > 0 {
			err = json.Unmarshal(response.Value, &evidence)
			if err != nil {
				return nil, err
			}
		}

		record := map[string]interface{}{
			"txId":      response.TxId,
			"timestamp": response.Timestamp,
			"isDelete":  response.IsDelete,
			"evidence":  evidence,
		}

		history = append(history, record)
	}

	return history, nil
}

func (c *ChainOfCustodyContract) GetAllEvidence(ctx contractapi.TransactionContextInterface) ([]*Evidence, error) {
	resultsIterator, err := ctx.GetStub().GetStateByRange("", "")
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	var evidences []*Evidence

	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(queryResponse.Key, "TRANSFER_") || queryResponse.Key == "EVIDENCE_INDEX" {
			continue
		}

		var evidence Evidence
		err = json.Unmarshal(queryResponse.Value, &evidence)
		if err != nil {
			continue
		}

		if evidence.ID == "" {
			continue
		}

		evidences = append(evidences, &evidence)
	}

	return evidences, nil
}

func (c *ChainOfCustodyContract) EvidenceExists(ctx contractapi.TransactionContextInterface, id string) (bool, error) {
	evidenceJSON, err := ctx.GetStub().GetState(id)
	if err != nil {
		return false, fmt.Errorf("failed to read evidence: %v", err)
	}

	return evidenceJSON != nil, nil
}

func (c *ChainOfCustodyContract) addToEvidenceIndex(ctx contractapi.TransactionContextInterface, id string) error {
	indexKey := "EVIDENCE_INDEX"
	indexJSON, err := ctx.GetStub().GetState(indexKey)
	
	var index []string
	if err != nil {
		return err
	}
	if indexJSON != nil {
		err = json.Unmarshal(indexJSON, &index)
		if err != nil {
			return err
		}
	}
	
	for _, existingID := range index {
		if existingID == id {
			return nil 
		}
	}
	
	index = append(index, id)
	updatedJSON, err := json.Marshal(index)
	if err != nil {
		return err
	}
	
	return ctx.GetStub().PutState(indexKey, updatedJSON)
}

func (c *ChainOfCustodyContract) GetAllEvidenceIDs(ctx contractapi.TransactionContextInterface) ([]string, error) {
	indexKey := "EVIDENCE_INDEX"
	indexJSON, err := ctx.GetStub().GetState(indexKey)
	if err != nil {
		return nil, err
	}
	if indexJSON == nil {
		return []string{}, nil
	}
	
	var index []string
	err = json.Unmarshal(indexJSON, &index)
	if err != nil {
		return nil, err
	}
	
	return index, nil
}

func (c *ChainOfCustodyContract) DeleteEvidence(ctx contractapi.TransactionContextInterface, id string) error {
	exists, err := c.EvidenceExists(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("evidence %s does not exist", id)
	}

	return ctx.GetStub().DelState(id)
}

func main() {
	chaincode, err := contractapi.NewChaincode(&ChainOfCustodyContract{})
	if err != nil {
		fmt.Printf("Error creating chain of custody chaincode: %v\n", err)
		return
	}

	if err := chaincode.Start(); err != nil {
		fmt.Printf("Error starting chain of custody chaincode: %v\n", err)
	}
}
