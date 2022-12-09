package vcluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

var distroKinds = []string{"k0s", "k8s", "k3s"}

const LoftChartRepo = "https://charts.loft.sh"

func resourceVCluster() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceVClusterCreate,
		ReadContext:   resourceVClusterRead,
		UpdateContext: resourceVClusterUpdate,
		DeleteContext: resourceVClusterDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:        schema.TypeString,
				Description: "The name of the vcluster",
				Required:    true,
				ForceNew:    true,
			},
			"distro": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringInSlice(distroKinds, true),
				ForceNew:     true,
			},
			"extra_values": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "List of values in raw yaml format to pass to vcluster.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"chart": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The virtual cluster chart name to use",
			},
			"chart_version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The virtual cluster chart version to use (e.g. v0.9.1)",
			},
			"chart_repo": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The virtual cluster chart repo to use",
			},
			"local_chart_dir": {
				Type:          schema.TypeString,
				ConflictsWith: []string{"chart", "chart_version", "chart_repo"},
				Optional:      true,
				Description:   "The virtual cluster local chart dir to use",
			},
			"kubernetes_version": {
				Type:        schema.TypeString,
				Description: "The kubernetes version to use (e.g. v1.20). Patch versions are not supported",
				Optional:    true,
			},
			"create_namespace": {
				Type:        schema.TypeBool,
				Description: "If true the namespace will be created if it does not exist",
				Optional:    true,
			},
			"disable_ingress_sync": {
				Type:        schema.TypeBool,
				Description: "If true the virtual cluster will not sync any ingresses",
				Optional:    true,
			},
			// UpdateCurrent?
			"expose": {
				Type:        schema.TypeBool,
				Description: "If true will create a load balancer service to expose the vcluster endpoint",
				Optional:    true,
			},
			"expose_local": {
				Type:        schema.TypeBool,
				Description: "If true and a local Kubernetes distro is detected, will deploy vcluster with a NodePort service",
				Optional:    true,
			},
			"isolate": {
				Type:        schema.TypeBool,
				Description: "If true vcluster and its workloads will run in an isolated environment",
				Optional:    true,
			},
			"context": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The kubernetes config context to use",
			},
			"namespace": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The kubernetes namespace to use",
			},
			"status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"created": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func vclusterBaseArgs(d *schema.ResourceData, args []string) []string {
	if namespace := d.Get("namespace"); namespace != nil && namespace.(string) != "" {
		args = append(args, "--namespace", namespace.(string))
	}

	if context := d.Get("context"); context != nil && context.(string) != "" {
		args = append(args, "--context", context.(string))
	}

	return args
}

func resourceVClusterCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	provider := m.(*Meta)
	_ = provider

	vClusterName := d.Get("name").(string)

	args := vclusterBaseArgs(d, []string{
		"create",
		vClusterName,
		"--connect=false",
	})

	if distro := d.Get("distro"); distro != nil && distro.(string) != "" {
		args = append(args, fmt.Sprintf("--distro=%s", distro.(string)))
	}

	if isolate := d.Get("isolate"); isolate != nil {
		args = append(args, fmt.Sprintf("--isolate=%v", isolate.(bool)))
	}

	if expose := d.Get("expose"); expose != nil {
		args = append(args, fmt.Sprintf("--expose=%v", expose.(bool)))
	}

	if exposeLocal := d.Get("expose_local"); exposeLocal != nil {
		args = append(args, fmt.Sprintf("--expose-local=%v", exposeLocal.(bool)))
	}

	if disableIngressSync := d.Get("disable_ingress_sync"); disableIngressSync != nil {
		args = append(args, fmt.Sprintf("--disable-ingress-sync=%v", disableIngressSync.(bool)))
	}

	if createNamespace := d.Get("create_namespace"); createNamespace != nil {
		args = append(args, fmt.Sprintf("--create-namespace=%v", createNamespace.(bool)))
	}

	if kubernetesVersion := d.Get("kubernetes_version"); kubernetesVersion != nil && kubernetesVersion.(string) != "" {
		args = append(args, fmt.Sprintf("--kubernetes-version=%s", kubernetesVersion.(string)))
	}

	// extra_values
	// chart
	// chart_version
	// chart_repo
	// local_chart_dir

	cmd := exec.Command(
		"vcluster",
		args...,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return diag.Diagnostics{
			{
				Severity: diag.Error,
				Summary:  fmt.Sprint("vcluster ", strings.Join(args, " ")),
				Detail:   string(output),
			},
		}
	}

	_ = output
	d.SetId(vClusterName)
	d.Set("name", vClusterName)

	return nil
}

// ListEntry is a struct matching the results of the vcluster list operation's json output.
type ListEntry struct {
	Name      string
	Namespace string
	Status    string
	Created   time.Time // "2022-12-09T03:12:10Z",
	Context   string
}

func resourceVClusterRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	args := vclusterBaseArgs(d, []string{
		"list",
		"--output", "json",
	})

	cmd := exec.Command(
		"vcluster",
		args...,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return diag.Diagnostics{
			{
				Severity: diag.Error,
				Summary:  fmt.Sprint("vcluster ", strings.Join(args, " ")),
				Detail:   string(output),
			},
		}
	}

	var entries []ListEntry
	err = json.Unmarshal(output, &entries)
	if err != nil {
		return diag.FromErr(err)
	}

	var resourceEntry ListEntry
	for _, entry := range entries {

		if entry.Name == d.Id() {
			resourceEntry = entry
		}
	}

	if resourceEntry == (ListEntry{}) {
		d.SetId("")
		return nil
	}

	d.Set("name", resourceEntry.Name)
	d.Set("status", resourceEntry.Status)
	d.Set("created", resourceEntry.Created)
	return nil
}

func resourceVClusterUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return nil
}

func resourceVClusterDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	args := vclusterBaseArgs(d, []string{
		"delete",
		d.Get("name").(string),
	})

	cmd := exec.Command(
		"vcluster",
		args...,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return diag.Diagnostics{
			{
				Severity: diag.Error,
				Summary:  fmt.Sprint("vcluster ", strings.Join(args, " ")),
				Detail:   string(output),
			},
		}
	}

	_ = output

	return nil
}
