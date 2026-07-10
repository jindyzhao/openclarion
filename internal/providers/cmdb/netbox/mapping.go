package netbox

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/openclarion/openclarion/internal/providers/cmdb/internal/cmdbnorm"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func (p *Provider) mapDevice(in deviceResponse) (ports.CMDBResource, error) {
	if in.ID <= 0 {
		return ports.CMDBResource{}, fmt.Errorf("id must be positive")
	}
	attributes := baseAttributes("dcim.device", in.ID)
	addRefAttribute(attributes, "netbox.role", in.Role)
	addRefAttribute(attributes, "netbox.tenant", in.Tenant)
	addRefAttribute(attributes, "netbox.platform", in.Platform)
	addRefAttribute(attributes, "netbox.site", in.Site)
	addRefAttribute(attributes, "netbox.location", in.Location)
	addRefAttribute(attributes, "netbox.rack", in.Rack)
	addRefAttribute(attributes, "netbox.cluster", in.Cluster)
	if err := addChoiceAttribute(attributes, "netbox.status", in.Status); err != nil {
		return ports.CMDBResource{}, fmt.Errorf("status: %w", err)
	}
	addIPAddressAttribute(attributes, in.PrimaryIP)
	addTagsAttribute(attributes, in.Tags)
	if err := p.addCustomAttributes(attributes, in.CustomFields); err != nil {
		return ports.CMDBResource{}, err
	}
	topology, err := topologyLinks(
		topologySpec{relation: "located_at", objectType: "dcim.site", ref: in.Site},
		topologySpec{relation: "located_at", objectType: "dcim.location", ref: in.Location},
		topologySpec{relation: "installed_in", objectType: "dcim.rack", ref: in.Rack},
		topologySpec{relation: "member_of", objectType: "virtualization.cluster", ref: in.Cluster},
	)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	return ports.CMDBResource{
		ID:         netBoxObjectID("dcim.device", in.ID),
		Kind:       string(ObjectTypeDevice),
		Name:       firstNonEmpty(in.Name, in.Display),
		Topology:   topology,
		Attributes: attributes,
	}, nil
}

func (p *Provider) mapVirtualMachine(in virtualMachineResponse) (ports.CMDBResource, error) {
	if in.ID <= 0 {
		return ports.CMDBResource{}, fmt.Errorf("id must be positive")
	}
	attributes := baseAttributes("virtualization.virtualmachine", in.ID)
	addRefAttribute(attributes, "netbox.role", in.Role)
	addRefAttribute(attributes, "netbox.tenant", in.Tenant)
	addRefAttribute(attributes, "netbox.platform", in.Platform)
	addRefAttribute(attributes, "netbox.site", in.Site)
	addRefAttribute(attributes, "netbox.cluster", in.Cluster)
	addRefAttribute(attributes, "netbox.device", in.Device)
	if err := addChoiceAttribute(attributes, "netbox.status", in.Status); err != nil {
		return ports.CMDBResource{}, fmt.Errorf("status: %w", err)
	}
	addIPAddressAttribute(attributes, in.PrimaryIP)
	addTagsAttribute(attributes, in.Tags)
	if err := p.addCustomAttributes(attributes, in.CustomFields); err != nil {
		return ports.CMDBResource{}, err
	}
	topology, err := topologyLinks(
		topologySpec{relation: "located_at", objectType: "dcim.site", ref: in.Site},
		topologySpec{relation: "member_of", objectType: "virtualization.cluster", ref: in.Cluster},
		topologySpec{relation: "hosted_on", objectType: "dcim.device", ref: in.Device},
	)
	if err != nil {
		return ports.CMDBResource{}, err
	}
	return ports.CMDBResource{
		ID:         netBoxObjectID("virtualization.virtualmachine", in.ID),
		Kind:       string(ObjectTypeVirtualMachine),
		Name:       firstNonEmpty(in.Name, in.Display),
		Topology:   topology,
		Attributes: attributes,
	}, nil
}

func (p *Provider) addCustomAttributes(attributes map[string]string, customFields map[string]json.RawMessage) error {
	for _, name := range p.attributeCustomFields {
		raw, ok := customFields[name]
		if !ok {
			continue
		}
		value, present, err := scalarString(raw)
		if err != nil {
			return fmt.Errorf("custom field %q must be a string, number, boolean, or null: %w", name, err)
		}
		if present && value != "" {
			attributes["netbox.custom."+name] = value
		}
	}
	return nil
}

func baseAttributes(objectType string, id int64) map[string]string {
	return map[string]string{
		"netbox.id":          strconv.FormatInt(id, 10),
		"netbox.object_type": objectType,
	}
}

func addRefAttribute(attributes map[string]string, key string, ref *objectReference) {
	if value := refName(ref); value != "" {
		attributes[key] = value
	}
}

func addChoiceAttribute(attributes map[string]string, key string, raw json.RawMessage) error {
	value, err := choiceString(raw)
	if err != nil {
		return err
	}
	if value != "" {
		attributes[key] = value
	}
	return nil
}

func addIPAddressAttribute(attributes map[string]string, ref *ipAddressReference) {
	if ref != nil && ref.Address != "" {
		attributes["netbox.primary_ip"] = ref.Address
	}
}

func addTagsAttribute(attributes map[string]string, tags []tagReference) {
	names := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag.Name == "" {
			continue
		}
		if _, ok := seen[tag.Name]; ok {
			continue
		}
		seen[tag.Name] = struct{}{}
		names = append(names, tag.Name)
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	value := strings.Join(names, ",")
	if len(value) > cmdbnorm.MaxStringBytes {
		return
	}
	attributes["netbox.tags"] = value
}

type topologySpec struct {
	relation   string
	objectType string
	ref        *objectReference
}

func topologyLinks(specs ...topologySpec) ([]ports.CMDBTopologyLink, error) {
	links := make([]ports.CMDBTopologyLink, 0, len(specs))
	for _, spec := range specs {
		if spec.ref == nil {
			continue
		}
		if spec.ref.ID <= 0 {
			return nil, fmt.Errorf("topology %s target id must be positive", spec.relation)
		}
		links = append(links, ports.CMDBTopologyLink{
			Relation:   spec.relation,
			TargetID:   netBoxObjectID(spec.objectType, spec.ref.ID),
			TargetKind: spec.objectType,
			TargetName: refName(spec.ref),
		})
	}
	return links, nil
}

func refName(ref *objectReference) string {
	if ref == nil {
		return ""
	}
	return firstNonEmpty(ref.Name, ref.Display)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func netBoxObjectID(objectType string, id int64) string {
	return "netbox:" + objectType + ":" + strconv.FormatInt(id, 10)
}
