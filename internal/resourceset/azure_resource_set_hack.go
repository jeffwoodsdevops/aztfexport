package resourceset

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"

	"github.com/tidwall/gjson"
)

// TweakResources tweaks the resource set exported from Azure, due to Terraform models the resources differently.
func (rset *AzureResourceSet) TweakResources() error {
	// KeyVault certificate is a special resource that its data plane entity is composed of two control plane resources.
	// Azure exports the control plane resource ids, while Terraform uses its data plane counterpart.
	if err := rset.tweakForKeyVaultCertificate(); err != nil {
		return err
	}

	// Populate managed data disk for VMs that are missing from Azure exported resource set.
	if err := rset.populateVMDataDisks(); err != nil {
		return err
	}

	return nil
}

func (rset *AzureResourceSet) tweakForKeyVaultCertificate() error {
	newResoruces := []AzureResource{}
	pending := map[string]AzureResource{}
	for _, res := range rset.Resources {
		if !strings.EqualFold(res.Id.RouteScopeString(), "/Microsoft.KeyVault/vaults/keys") && !strings.EqualFold(res.Id.RouteScopeString(), "/Microsoft.KeyVault/vaults/secrets") {
			newResoruces = append(newResoruces, res)
			continue
		}
		names := res.Id.Names()
		certName := names[len(names)-1]
		if _, ok := pending[certName]; !ok {
			pending[certName] = res
			continue
		}
		delete(pending, certName)
		certId := res.Id.Clone().(*armid.ScopedResourceId)
		certId.AttrTypes[len(certId.AttrTypes)-1] = "certificates"
		newResoruces = append(newResoruces, AzureResource{
			Id: certId,
		})
	}
	for _, res := range pending {
		newResoruces = append(newResoruces, res)
	}
	rset.Resources = newResoruces
	return nil
}

func (rset *AzureResourceSet) populateVMDataDisks() error {
	for _, res := range rset.Resources[:] {
		if strings.ToUpper(res.Id.RouteScopeString()) != "/MICROSOFT.COMPUTE/VIRTUALMACHINES" {
			continue
		}
		disks, err := populateManagedResourcesByPath(res, "properties.storageProfile.dataDisks.#.managedDisk.id")
		if err != nil {
			return fmt.Errorf(`populating managed disks for %q: %v`, res.Id, err)
		}
		rset.Resources = append(rset.Resources, disks...)

		// Add the association resource
		for _, disk := range disks {
			diskName := disk.Id.Names()[0]

			// It doesn't matter using linux/windows below, as their resource ids are the same.
			vmTFId, err := aztft.QueryId(res.Id.String(), "azurerm_linux_virtual_machine", false)
			if err != nil {
				return fmt.Errorf("querying resource id for %s: %v", res.Id, err)
			}

			azureId := res.Id.Clone().(*armid.ScopedResourceId)
			azureId.AttrTypes = append(azureId.AttrTypes, "dataDisks")
			azureId.AttrNames = append(azureId.AttrNames, diskName)

			rset.Resources = append(rset.Resources, AzureResource{
				Id: azureId,
				PesudoResourceInfo: &PesudoResourceInfo{
					TFType: "azurerm_virtual_machine_data_disk_attachment",
					TFId:   vmTFId + "/dataDisks/" + diskName,
				},
			})
		}
	}
	return nil
}

// populateManagedResourcesByPath populate the managed resources in the specified paths.
func populateManagedResourcesByPath(res AzureResource, paths ...string) ([]AzureResource, error) {
	b, err := json.Marshal(res.Properties)
	if err != nil {
		return nil, fmt.Errorf("marshaling %v: %v", res.Properties, err)
	}
	var resources []AzureResource
	for _, path := range paths {
		result := gjson.GetBytes(b, path)
		if !result.Exists() {
			continue
		}

		for _, exprResult := range result.Array() {
			mid := exprResult.String()
			id, err := armid.ParseResourceId(mid)
			if err != nil {
				return nil, fmt.Errorf("parsing managed resource id %s: %v", mid, err)
			}
			resources = append(resources, AzureResource{
				Id: id,
			})
		}
	}
	return resources, nil
}
