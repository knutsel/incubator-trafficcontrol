/*

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

package v13

import (
	"fmt"
	"testing"
	"time"

	"github.com/apache/incubator-trafficcontrol/lib/go-log"
	tc "github.com/apache/incubator-trafficcontrol/lib/go-tc"
)

func TestParameters(t *testing.T) {

	toReqTimeout := time.Second * time.Duration(Config.Default.Session.TimeoutInSecs)
	//TOSession, _, err = SetupSession(toReqTimeout, Config.TrafficOps.URL, Config.TrafficOps.User, Config.TrafficOps.UserPassword)
	//if err != nil {
	//t.Errorf("could not CREATE session: %v\n", err)
	//}
	TeardownSession(toReqTimeout, Config.TrafficOps.URL, Config.TrafficOps.AdminUser, Config.TrafficOps.UserPassword)
	SetupSession(toReqTimeout, Config.TrafficOps.URL, Config.TrafficOps.PortalUser, Config.TrafficOps.UserPassword)

	CreateTestParameters(t)
	UpdateTestParameters(t)
	GetTestParameters(t)
	//DeleteTestParameters(t)

}

func CreateTestParameters(t *testing.T) {

	for _, pl := range testData.Parameters {
		resp, _, err := TOSession.CreateParameter(pl)
		log.Debugln("Response: ", resp)
		if err != nil {
			t.Errorf("could not CREATE phys_locations: %v\n", err)
		}
	}

}

func UpdateTestParameters(t *testing.T) {

	firstParameter := testData.Parameters[0]
	// Retrieve the Parameter by name so we can get the id for the Update
	resp, _, err := TOSession.GetParameterByName(firstParameter.Name)
	if err != nil {
		t.Errorf("cannot GET Parameter by name: %v - %v\n", firstParameter.Name, err)
	}
	remoteParameter := resp[0]
	expectedParameterName := "UPDATED"
	remoteParameter.Name = expectedParameterName
	var alert tc.Alerts
	alert, _, err = TOSession.UpdateParameterByID(remoteParameter.ID, remoteParameter)
	if err != nil {
		t.Errorf("cannot UPDATE Parameter by id: %v - %v\n", err, alert)
	}

	// Retrieve the Parameter to check Parameter name got updated
	resp, _, err = TOSession.GetParameterByID(remoteParameter.ID)
	if err != nil {
		t.Errorf("cannot GET Parameter by name: %v - %v\n", firstParameter.Name, err)
	}
	respParameter := resp[0]
	if respParameter.Name != expectedParameterName {
		t.Errorf("results do not match actual: %s, expected: %s\n", respParameter.Name, expectedParameterName)
	}

}

func GetTestParameters(t *testing.T) {

	for _, pl := range testData.Parameters {
		resp, _, err := TOSession.GetParameterByName(pl.Name)
		fmt.Printf("resp ---> %v\n", resp)
		if err != nil {
			t.Errorf("cannot GET Parameter by name: %v - %v\n", err, resp)
		}
	}
}

func DeleteTestParameters(t *testing.T) {

	for _, pl := range testData.Parameters {
		// Retrieve the Parameter by name so we can get the id for the Update
		resp, _, err := TOSession.GetParameterByNameAndConfigFile(pl.Name, pl.ConfigFile)
		if err != nil {
			t.Errorf("cannot GET Parameter by name: %v - %v\n", pl.Name, err)
		}
		if len(resp) > 0 {
			respParameter := resp[0]

			delResp, _, err := TOSession.DeleteParameterByID(respParameter.ID)
			if err != nil {
				t.Errorf("cannot DELETE Parameter by name: %v - %v\n", err, delResp)
			}
			//time.Sleep(1 * time.Second)

			// Retrieve the Parameter to see if it got deleted
			pls, _, err := TOSession.GetParameterByNameAndConfigFile(pl.Name, pl.ConfigFile)
			if err != nil {
				t.Errorf("error deleting Parameter name: %s\n", err.Error())
			}
			if len(pls) > 0 {
				t.Errorf("expected Parameter Name: %s and ConfigFile: %s to be deleted\n", pl.Name, pl.ConfigFile)
			}
		}
	}
}