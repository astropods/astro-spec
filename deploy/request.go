package deploy

// TemplateRequest is the part of astro-server's deployment-template request
// body that an environment sets. The caller adds the build, the deployment,
// and the target account.
type TemplateRequest struct {
	ClusterID    string                   `json:"cluster_id,omitempty"`
	Interfaces   *TemplateInterfaces      `json:"interfaces,omitempty"`
	Variables    map[string]VariableInput `json:"variables,omitempty"`
	Models       map[string]string        `json:"models,omitempty"`
	Schedules    map[string]string        `json:"schedules,omitempty"`
	Bindings     *Bindings                `json:"bindings,omitempty"`
	Provisioning *Provisioning            `json:"provisioning,omitempty"`
}

// TemplateInterfaces replaces the deployment's adapters with Adapters, so it
// is sent only when the environment names its interfaces.
type TemplateInterfaces struct {
	Adapters []string        `json:"adapters"`
	Auth     *InterfacesAuth `json:"auth,omitempty"`
}

type InterfacesAuth struct {
	Web    *PublicFlag `json:"web,omitempty"`
	Custom *PublicFlag `json:"custom,omitempty"`
}

type PublicFlag struct {
	Public bool `json:"public"`
}

type VariableInput struct {
	Value string `json:"value,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

type Bindings struct {
	Knowledge map[string]string `json:"knowledge,omitempty"`
}

type Provisioning struct {
	Agent        *ComponentProvisioning           `json:"agent,omitempty"`
	Models       map[string]ComponentProvisioning `json:"models,omitempty"`
	Knowledge    map[string]ComponentProvisioning `json:"knowledge,omitempty"`
	Integrations map[string]ComponentProvisioning `json:"integrations,omitempty"`
	Ingestion    map[string]ComponentProvisioning `json:"ingestion,omitempty"`
}

type ComponentProvisioning struct {
	Compute *ComponentCompute `json:"compute,omitempty"`
}

type ComponentCompute struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// TemplateRequest maps the environment onto a deployment-template request.
// A field the environment leaves out stays empty, so the server keeps the
// stored value on a redeploy and the default on a create.
func (e Environment) TemplateRequest() TemplateRequest {
	req := TemplateRequest{
		ClusterID: e.Cluster,
		Models:    e.Models,
		Schedules: e.Schedules,
	}
	if e.Interfaces != nil {
		req.Interfaces = &TemplateInterfaces{Adapters: e.Interfaces}
		if e.Public != nil {
			auth := &InterfacesAuth{}
			if e.Public.Web != nil {
				auth.Web = &PublicFlag{Public: *e.Public.Web}
			}
			if e.Public.Custom != nil {
				auth.Custom = &PublicFlag{Public: *e.Public.Custom}
			}
			req.Interfaces.Auth = auth
		}
	}
	if len(e.Variables) > 0 {
		req.Variables = make(map[string]VariableInput, len(e.Variables))
		for name, v := range e.Variables {
			req.Variables[name] = VariableInput(v)
		}
	}
	if len(e.Knowledge) > 0 {
		req.Bindings = &Bindings{Knowledge: e.Knowledge}
	}
	if e.Resources != nil {
		req.Provisioning = e.Resources.provisioning()
	}
	return req
}

func (r Resources) provisioning() *Provisioning {
	p := &Provisioning{
		Models:       componentMap(r.Models),
		Knowledge:    componentMap(r.Knowledge),
		Integrations: componentMap(r.Integrations),
		Ingestion:    componentMap(r.Ingestion),
	}
	if r.Agent != nil {
		c := component(*r.Agent)
		p.Agent = &c
	}
	return p
}

func componentMap(in map[string]Compute) map[string]ComponentProvisioning {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]ComponentProvisioning, len(in))
	for name, c := range in {
		out[name] = component(c)
	}
	return out
}

func component(c Compute) ComponentProvisioning {
	return ComponentProvisioning{Compute: &ComponentCompute{CPU: c.CPU, Memory: c.Memory}}
}
