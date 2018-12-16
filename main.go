package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/alexellis/ofc-bootstrap/pkg/execute"
	"github.com/alexellis/ofc-bootstrap/pkg/ingress"
	"github.com/alexellis/ofc-bootstrap/pkg/stack"
	"github.com/alexellis/ofc-bootstrap/pkg/types"
	yaml "gopkg.in/yaml.v2"
)

type Vars struct {
	YamlFile string
	Verbose  bool
}

const (
	OrchestrationK8s   = "kubernetes"
	OrchestrationSwarm = "swarm"
)

func main() {

	vars := Vars{}
	flag.StringVar(&vars.YamlFile, "yaml", "", "YAML file for bootstrap")
	flag.BoolVar(&vars.Verbose, "verbose", false, "control verbosity")
	flag.Parse()

	if len(vars.YamlFile) == 0 {
		fmt.Fprintf(os.Stderr, "No -yaml flag given\n")
		os.Exit(1)
	}

	yamlBytes, yamlErr := ioutil.ReadFile(vars.YamlFile)
	if yamlErr != nil {
		fmt.Fprintf(os.Stderr, "-yaml file gave error: %s\n", yamlErr.Error())
		os.Exit(1)
	}

	plan := types.Plan{}
	unmarshalErr := yaml.Unmarshal(yamlBytes, &plan)
	if unmarshalErr != nil {
		fmt.Fprintf(os.Stderr, "-yaml file gave error: %s\n", unmarshalErr.Error())
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "Plan loaded from: %s\n", vars.YamlFile)

	start := time.Now()
	err := process(plan)
	done := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stdout, "Plan failed after %f seconds\nError: %s", done.Seconds(), err.Error())

		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "Plan completed in %f seconds\n", done.Seconds())
}

func process(plan types.Plan) error {

	fmt.Println(plan)
	if plan.Orchestration == OrchestrationK8s {
		fmt.Println("Orchestration: Kubernetes")
	} else if plan.Orchestration == OrchestrationSwarm {
		fmt.Println("Orchestration: Swarm")
	}

	createSecrets(plan)

	if plan.Orchestration == OrchestrationK8s {
		fmt.Println("Building Ingress")

		nsErr := createNamespaces()
		if nsErr != nil {
			log.Println(nsErr)
		}

		cmErr := installCertmanager()
		if cmErr != nil {
			log.Println(cmErr)
		}

		ofErr := installOpenfaas()
		if ofErr != nil {
			log.Println(ofErr)
		}

		ingressErr := ingress.Apply(plan)
		if ingressErr != nil {
			log.Println(ingressErr)
		}

		fmt.Println("Creating stack.yml")

		planErr := stack.Apply(plan)
		if planErr != nil {
			return planErr
		}

	}

	return nil
}

func installIngressController() error {
	log.Println("Creating Ingress Controller")

	task := execute.ExecTask{
		Command: "scripts/install-nginx.sh",
		Shell:   true,
	}

	res, err := task.Execute()

	if err != nil {
		return err
	}

	log.Println(res.Stdout)

	return nil
}

func installOpenfaas() error {
	log.Println("Creating OpenFaaS")

	task := execute.ExecTask{
		Command: "scripts/install-openfaas.sh",
		Shell:   true,
	}

	res, err := task.Execute()

	if err != nil {
		return err
	}

	log.Println(res.Stdout)

	return nil
}

func installCertmanager() error {
	log.Println("Creating Cert-Manager")

	task := execute.ExecTask{
		Command: "scripts/install-certmanager.sh",
		Shell:   true,
	}

	res, err := task.Execute()

	if err != nil {
		return err
	}

	log.Println(res.Stdout)

	return nil
}

func createNamespaces() error {
	log.Println("Creating namespaces")

	task := execute.ExecTask{
		Command: "scripts/create-namespaces.sh",
		Shell:   true,
	}

	res, err := task.Execute()

	if err != nil {
		return err
	}

	log.Println(res.Stdout)

	return nil
}

func createSecrets(plan types.Plan) error {

	fmt.Println(plan.Secrets)

	var command execute.ExecTask
	if plan.Orchestration == OrchestrationK8s {
		command = execute.ExecTask{Command: createK8sSecret(plan.Secrets[0])}
	} else if plan.Orchestration == OrchestrationSwarm {
		command = execute.ExecTask{Command: createDockerSecret(plan.Secrets[0])}
	}

	res, err := command.Execute()
	fmt.Println(res)

	return err
}

func generateSecret() (string, error) {
	task := execute.ExecTask{
		Command: "head -c 16 /dev/urandom | shasum",
		Shell:   true,
	}

	res, err := task.Execute()
	if res.ExitCode != 0 && err != nil {
		err = fmt.Errorf("non-zero exit code")
	}

	return res.Stdout, err
}

func createDockerSecret(kvn types.KeyValueNamespaceTuple) string {
	val, err := generateSecret()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return fmt.Sprintf("echo %s | docker secret create %s", val, kvn.Name)
}

func createK8sSecret(kvn types.KeyValueNamespaceTuple) string {
	val, err := generateSecret()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return fmt.Sprintf("kubectl create secret generic -n %s %s --from-literal s3-access-key=\"%s\"", kvn.Namespace, kvn.Name, val)
}
