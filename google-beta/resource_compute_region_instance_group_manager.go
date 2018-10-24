package google

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"

	computeBeta "google.golang.org/api/compute/v0.beta"
)

var (
	regionInstanceGroupManagerIdRegex     = regexp.MustCompile("^" + ProjectRegex + "/[a-z0-9-]+/[a-z0-9-]+$")
	regionInstanceGroupManagerIdNameRegex = regexp.MustCompile("^[a-z0-9-]+$")
)

func resourceComputeRegionInstanceGroupManager() *schema.Resource {
	return &schema.Resource{
		Create: resourceComputeRegionInstanceGroupManagerCreate,
		Read:   resourceComputeRegionInstanceGroupManagerRead,
		Update: resourceComputeRegionInstanceGroupManagerUpdate,
		Delete: resourceComputeRegionInstanceGroupManagerDelete,
		Importer: &schema.ResourceImporter{
			State: resourceRegionInstanceGroupManagerStateImporter,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
			Delete: schema.DefaultTimeout(15 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"base_instance_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"version": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"instance_template": &schema.Schema{
							Type:             schema.TypeString,
							Required:         true,
							DiffSuppressFunc: compareSelfLinkRelativePaths,
						},

						"target_size": &schema.Schema{
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"fixed": &schema.Schema{
										Type:     schema.TypeInt,
										Optional: true,
									},

									"percent": &schema.Schema{
										Type:         schema.TypeInt,
										Optional:     true,
										ValidateFunc: validation.IntBetween(0, 100),
									},
								},
							},
						},
					},
				},
			},

			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"region": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"description": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"fingerprint": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"instance_group": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"named_port": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"port": &schema.Schema{
							Type:     schema.TypeInt,
							Required: true,
						},
					},
				},
			},

			"project": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},

			"self_link": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},

			"target_pools": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Set: selfLinkRelativePathHash,
			},
			"target_size": &schema.Schema{
				Type:     schema.TypeInt,
				Computed: true,
				Optional: true,
			},

			// If true, the resource will report ready only after no instances are being created.
			// This will not block future reads if instances are being recreated, and it respects
			// the "createNoRetry" parameter that's available for this resource.
			"wait_for_instances": &schema.Schema{
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"auto_healing_policies": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"health_check": &schema.Schema{
							Type:             schema.TypeString,
							Required:         true,
							DiffSuppressFunc: compareSelfLinkRelativePaths,
						},

						"initial_delay_sec": &schema.Schema{
							Type:         schema.TypeInt,
							Required:     true,
							ValidateFunc: validation.IntBetween(0, 3600),
						},
					},
				},
			},

			"distribution_policy_zones": &schema.Schema{
				Type:     schema.TypeSet,
				Optional: true,
				ForceNew: true,
				Computed: true,
				Set:      hashZoneFromSelfLinkOrResourceName,
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					DiffSuppressFunc: compareSelfLinkOrResourceName,
				},
			},

			"update_policy": &schema.Schema{
				Computed: true,
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"minimal_action": &schema.Schema{
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringInSlice([]string{"RESTART", "REPLACE"}, false),
						},

						"type": &schema.Schema{
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringInSlice([]string{"OPPORTUNISTIC", "PROACTIVE"}, false),
						},

						"max_surge_fixed": &schema.Schema{
							Type:          schema.TypeInt,
							Optional:      true,
							Computed:      true,
							ConflictsWith: []string{"update_policy.0.max_surge_percent"},
						},

						"max_surge_percent": &schema.Schema{
							Type:          schema.TypeInt,
							Optional:      true,
							ConflictsWith: []string{"update_policy.0.max_surge_fixed"},
							ValidateFunc:  validation.IntBetween(0, 100),
						},

						"max_unavailable_fixed": &schema.Schema{
							Type:          schema.TypeInt,
							Optional:      true,
							Computed:      true,
							ConflictsWith: []string{"update_policy.0.max_unavailable_percent"},
						},

						"max_unavailable_percent": &schema.Schema{
							Type:          schema.TypeInt,
							Optional:      true,
							ConflictsWith: []string{"update_policy.0.max_unavailable_fixed"},
							ValidateFunc:  validation.IntBetween(0, 100),
						},

						"min_ready_sec": &schema.Schema{
							Type:         schema.TypeInt,
							Optional:     true,
							ValidateFunc: validation.IntBetween(0, 3600),
						},
					},
				},
			},
		},
	}
}

func resourceComputeRegionInstanceGroupManagerCreate(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)

	project, err := getProject(d, config)
	if err != nil {
		return err
	}

	region, err := getRegion(d, config)
	if err != nil {
		return err
	}

	manager := &computeBeta.InstanceGroupManager{
		Name:                d.Get("name").(string),
		Description:         d.Get("description").(string),
		BaseInstanceName:    d.Get("base_instance_name").(string),
		TargetSize:          int64(d.Get("target_size").(int)),
		NamedPorts:          getNamedPortsBeta(d.Get("named_port").(*schema.Set).List()),
		TargetPools:         convertStringSet(d.Get("target_pools").(*schema.Set)),
		AutoHealingPolicies: expandAutoHealingPolicies(d.Get("auto_healing_policies").([]interface{})),
		Versions:            expandVersions(d.Get("version").([]interface{})),
		UpdatePolicy:        expandUpdatePolicy(d.Get("update_policy").([]interface{})),
		DistributionPolicy:  expandDistributionPolicy(d.Get("distribution_policy_zones").(*schema.Set)),
		// Force send TargetSize to allow size of 0.
		ForceSendFields: []string{"TargetSize"},
	}

	op, err := config.clientComputeBeta.RegionInstanceGroupManagers.Insert(project, region, manager).Do()

	if err != nil {
		return fmt.Errorf("Error creating RegionInstanceGroupManager: %s", err)
	}

	d.SetId(regionInstanceGroupManagerId{Project: project, Region: region, Name: manager.Name}.terraformId())

	// Wait for the operation to complete
	err = computeSharedOperationWait(config.clientCompute, op, project, "Creating InstanceGroupManager")
	if err != nil {
		return err
	}
	return resourceComputeRegionInstanceGroupManagerRead(d, config)
}

type getInstanceManagerFunc func(*schema.ResourceData, interface{}) (*computeBeta.InstanceGroupManager, error)

func getRegionalManager(d *schema.ResourceData, meta interface{}) (*computeBeta.InstanceGroupManager, error) {
	config := meta.(*Config)

	regionalID, err := parseRegionInstanceGroupManagerId(d.Id())
	if err != nil {
		return nil, err
	}

	if regionalID.Project == "" {
		regionalID.Project, err = getProject(d, config)
		if err != nil {
			return nil, err
		}
	}

	if regionalID.Region == "" {
		regionalID.Region, err = getRegion(d, config)
		if err != nil {
			return nil, err
		}
	}

	manager, err := config.clientComputeBeta.RegionInstanceGroupManagers.Get(regionalID.Project, regionalID.Region, regionalID.Name).Do()
	if err != nil {
		return nil, handleNotFoundError(err, d, fmt.Sprintf("Region Instance Manager %q", regionalID.Name))
	}

	return manager, nil
}

func waitForInstancesRefreshFunc(f getInstanceManagerFunc, d *schema.ResourceData, meta interface{}) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		m, err := f(d, meta)
		if err != nil {
			log.Printf("[WARNING] Error in fetching manager while waiting for instances to come up: %s\n", err)
			return nil, "error", err
		}
		if done := m.CurrentActions.None; done < m.TargetSize {
			return done, "creating", nil
		} else {
			return done, "created", nil
		}
	}
}

func resourceComputeRegionInstanceGroupManagerRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	manager, err := getRegionalManager(d, meta)
	if err != nil {
		return err
	}
	if manager == nil {
		log.Printf("[WARN] Region Instance Group Manager %q not found, removing from state.", d.Id())
		d.SetId("")
		return nil
	}

	regionalID, err := parseRegionInstanceGroupManagerId(d.Id())
	if err != nil {
		return err
	}
	if regionalID.Project == "" {
		regionalID.Project, err = getProject(d, config)
		if err != nil {
			return err
		}
	}

	d.Set("base_instance_name", manager.BaseInstanceName)
	if err := d.Set("version", flattenVersions(manager.Versions)); err != nil {
		return err
	}
	d.Set("name", manager.Name)
	d.Set("region", GetResourceNameFromSelfLink(manager.Region))
	d.Set("description", manager.Description)
	d.Set("project", regionalID.Project)
	d.Set("target_size", manager.TargetSize)
	if err := d.Set("target_pools", manager.TargetPools); err != nil {
		return fmt.Errorf("Error setting target_pools in state: %s", err.Error())
	}
	if err := d.Set("named_port", flattenNamedPortsBeta(manager.NamedPorts)); err != nil {
		return fmt.Errorf("Error setting named_port in state: %s", err.Error())
	}
	d.Set("fingerprint", manager.Fingerprint)
	d.Set("instance_group", ConvertSelfLinkToV1(manager.InstanceGroup))
	if err := d.Set("auto_healing_policies", flattenAutoHealingPolicies(manager.AutoHealingPolicies)); err != nil {
		return fmt.Errorf("Error setting auto_healing_policies in state: %s", err.Error())
	}
	if err := d.Set("distribution_policy_zones", flattenDistributionPolicy(manager.DistributionPolicy)); err != nil {
		return err
	}
	d.Set("self_link", ConvertSelfLinkToV1(manager.SelfLink))
	if err := d.Set("update_policy", flattenUpdatePolicy(manager.UpdatePolicy)); err != nil {
		return fmt.Errorf("Error setting update_policy in state: %s", err.Error())
	}

	if d.Get("wait_for_instances").(bool) {
		conf := resource.StateChangeConf{
			Pending: []string{"creating", "error"},
			Target:  []string{"created"},
			Refresh: waitForInstancesRefreshFunc(getRegionalManager, d, meta),
			Timeout: d.Timeout(schema.TimeoutCreate),
		}
		_, err := conf.WaitForState()
		if err != nil {
			return err
		}
	}

	return nil
}

func resourceComputeRegionInstanceGroupManagerUpdate(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)

	project, err := getProject(d, config)
	if err != nil {
		return err
	}

	region, err := getRegion(d, config)
	if err != nil {
		return err
	}

	updatedManager := &computeBeta.InstanceGroupManager{
		Fingerprint: d.Get("fingerprint").(string),
	}
	var change bool

	if d.HasChange("target_pools") {
		updatedManager.TargetPools = convertStringSet(d.Get("target_pools").(*schema.Set))
		change = true
	}

	if d.HasChange("auto_healing_policies") {
		updatedManager.AutoHealingPolicies = expandAutoHealingPolicies(d.Get("auto_healing_policies").([]interface{}))
		updatedManager.ForceSendFields = append(updatedManager.ForceSendFields, "AutoHealingPolicies")
		change = true
	}

	if d.HasChange("version") {
		updatedManager.Versions = expandVersions(d.Get("version").([]interface{}))
		change = true
	}

	if d.HasChange("update_policy") {
		updatedManager.UpdatePolicy = expandUpdatePolicy(d.Get("update_policy").([]interface{}))
		change = true
	}

	if change {
		op, err := config.clientComputeBeta.RegionInstanceGroupManagers.Patch(project, region, d.Get("name").(string), updatedManager).Do()
		if err != nil {
			return fmt.Errorf("Error updating region managed group instances: %s", err)
		}

		err = computeSharedOperationWait(config.clientCompute, op, project, "Updating region managed group instances")
		if err != nil {
			return err
		}
	}

	// named ports can't be updated through PATCH
	// so we call the update method on the region instance group, instead of the rigm
	if d.HasChange("named_port") {
		namedPorts := getNamedPortsBeta(d.Get("named_port").(*schema.Set).List())
		setNamedPorts := &computeBeta.RegionInstanceGroupsSetNamedPortsRequest{
			NamedPorts: namedPorts,
		}

		op, err := config.clientComputeBeta.RegionInstanceGroups.SetNamedPorts(
			project, region, d.Get("name").(string), setNamedPorts).Do()

		if err != nil {
			return fmt.Errorf("Error updating RegionInstanceGroupManager: %s", err)
		}

		err = computeSharedOperationWait(config.clientCompute, op, project, "Updating RegionInstanceGroupManager")
		if err != nil {
			return err
		}
	}

	// target size should use resize
	if d.HasChange("target_size") {
		targetSize := int64(d.Get("target_size").(int))
		op, err := config.clientComputeBeta.RegionInstanceGroupManagers.Resize(
			project, region, d.Get("name").(string), targetSize).Do()

		if err != nil {
			return fmt.Errorf("Error resizing RegionInstanceGroupManager: %s", err)
		}

		err = computeSharedOperationWait(config.clientCompute, op, project, "Resizing RegionInstanceGroupManager")
		if err != nil {
			return err
		}
	}

	return resourceComputeRegionInstanceGroupManagerRead(d, meta)
}

func resourceComputeRegionInstanceGroupManagerDelete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)

	regionalID, err := parseRegionInstanceGroupManagerId(d.Id())
	if err != nil {
		return err
	}

	if regionalID.Project == "" {
		regionalID.Project, err = getProject(d, config)
		if err != nil {
			return err
		}
	}

	if regionalID.Region == "" {
		regionalID.Region, err = getRegion(d, config)
		if err != nil {
			return err
		}
	}

	op, err := config.clientComputeBeta.RegionInstanceGroupManagers.Delete(regionalID.Project, regionalID.Region, regionalID.Name).Do()

	if err != nil {
		return fmt.Errorf("Error deleting region instance group manager: %s", err)
	}

	// Wait for the operation to complete
	err = computeSharedOperationWaitTime(config.clientCompute, op, regionalID.Project, int(d.Timeout(schema.TimeoutDelete).Minutes()), "Deleting RegionInstanceGroupManager")

	d.SetId("")
	return nil
}

func expandDistributionPolicy(configured *schema.Set) *computeBeta.DistributionPolicy {
	if configured.Len() == 0 {
		return nil
	}

	distributionPolicyZoneConfigs := make([]*computeBeta.DistributionPolicyZoneConfiguration, 0, configured.Len())
	for _, raw := range configured.List() {
		data := raw.(string)
		distributionPolicyZoneConfig := computeBeta.DistributionPolicyZoneConfiguration{
			Zone: "zones/" + data,
		}

		distributionPolicyZoneConfigs = append(distributionPolicyZoneConfigs, &distributionPolicyZoneConfig)
	}
	return &computeBeta.DistributionPolicy{Zones: distributionPolicyZoneConfigs}
}

func flattenDistributionPolicy(distributionPolicy *computeBeta.DistributionPolicy) []string {
	zones := make([]string, 0)

	if distributionPolicy != nil {
		for _, zone := range distributionPolicy.Zones {
			zones = append(zones, zone.Zone)
		}
	}

	return zones
}

func hashZoneFromSelfLinkOrResourceName(value interface{}) int {
	parts := strings.Split(value.(string), "/")
	resource := parts[len(parts)-1]

	return hashcode.String(resource)
}

func resourceRegionInstanceGroupManagerStateImporter(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	d.Set("wait_for_instances", false)
	regionalID, err := parseRegionInstanceGroupManagerId(d.Id())
	if err != nil {
		return nil, err
	}
	d.Set("project", regionalID.Project)
	d.Set("region", regionalID.Region)
	d.Set("name", regionalID.Name)
	return []*schema.ResourceData{d}, nil
}

type regionInstanceGroupManagerId struct {
	Project string
	Region  string
	Name    string
}

func (r regionInstanceGroupManagerId) terraformId() string {
	return fmt.Sprintf("%s/%s/%s", r.Project, r.Region, r.Name)
}

func parseRegionInstanceGroupManagerId(id string) (*regionInstanceGroupManagerId, error) {
	switch {
	case regionInstanceGroupManagerIdRegex.MatchString(id):
		parts := strings.Split(id, "/")
		return &regionInstanceGroupManagerId{
			Project: parts[0],
			Region:  parts[1],
			Name:    parts[2],
		}, nil
	case regionInstanceGroupManagerIdNameRegex.MatchString(id):
		return &regionInstanceGroupManagerId{
			Name: id,
		}, nil
	default:
		return nil, fmt.Errorf("Invalid region instance group manager specifier. Expecting either {projectId}/{region}/{name} or {name}, where {projectId} and {region} will be derived from the provider.")
	}
}
