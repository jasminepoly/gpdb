package services_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"gp_upgrade/hub/configutils"
	"gp_upgrade/hub/services"
	pb "gp_upgrade/idl"
	"gp_upgrade/utils"

	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/onsi/gomega/gbytes"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestHelperProcess isn't a real test. It's used as a helper process
// for TestParameterRun.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	mockedOutput := os.Getenv("MOCKED_OUTPUT")
	mockedExitStatus, err := strconv.Atoi(os.Getenv("MOCKED_EXIT_STATUS"))
	if err != nil {
		mockedOutput = "Exit status conversion failed.\nAre we missing the mocked_exit_status?"
		mockedExitStatus = -1
	}
	defer os.Exit(mockedExitStatus)
	fmt.Fprintf(os.Stdout, mockedOutput)
}

var _ = Describe("hub", func() {
	var (
		mockedOutput     string
		mockedExitStatus int

		testStdout *gbytes.Buffer
		testStdErr *gbytes.Buffer
	)

	/* This idea came from https://golang.org/src/os/exec/exec_test.go */
	fakeExecCommand := func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		output := fmt.Sprintf("MOCKED_OUTPUT=%s", mockedOutput)
		exitStatus := fmt.Sprintf("MOCKED_EXIT_STATUS=%d", mockedExitStatus)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", output, exitStatus}
		return cmd
	}

	BeforeEach(func() {
		testStdout, testStdErr, _ = testhelper.SetupTestLogger() // extend to capture the values in a var if future tests need it
	})

	Describe("ConvertMasterHub", func() {
		It("Sends that convert master started successfully", func() {
			mockedExitStatus = 0
			mockedOutput = `pg_upgrade running conversion:
	Some pg_upgrade output here
	Passed through all of pg_upgrade`

			reader := configutils.NewReader()
			dir, err := ioutil.TempDir("", "")
			defer os.RemoveAll(dir)
			Expect(err).ToNot(HaveOccurred())
			hub := services.NewHub(nil, &reader, grpc.DialContext, &services.HubConfig{
				StateDir: dir,
			})

			utils.System.ExecCommand = fakeExecCommand
			services.GetMasterDataDirs = func(baseDir string) (string, string, error) {
				return "old/datadirectory/path", "new/datadirectory/path", nil
			}
			defer func() { utils.System.ExecCommand = exec.Command }()

			fakeUpgradeConvertMasterRequest := &pb.UpgradeConvertMasterRequest{
				OldBinDir: "/old/path/bin",
				NewBinDir: "/new/path/bin"}

			_, err = hub.UpgradeConvertMaster(nil, fakeUpgradeConvertMasterRequest)
			Expect(err).ToNot(HaveOccurred())

			Eventually(testStdout).Should(gbytes.Say("Starting master upgrade"))
			Eventually(testStdErr).Should(gbytes.Say(""))
			Eventually(testStdout).Should(gbytes.Say("Found no errors when starting the upgrade"))
		})
		// This can't work because we don't have a good way to force a failure
		// for Start? Will need to find a good way.
		XIt("Sends a failure when pg_upgrade failed due to some issue", func() {
			mockedExitStatus = 1
			mockedOutput = `pg_upgrade exploded!
	Some kind of error message here that helps us understand what's going on
	Some kind of obscure error message`

			reader := configutils.NewReader()
			dir, err := ioutil.TempDir("", "")
			defer os.RemoveAll(dir)
			Expect(err).ToNot(HaveOccurred())
			hub := services.NewHub(nil, &reader, grpc.DialContext, &services.HubConfig{
				StateDir: dir,
			})

			utils.System.ExecCommand = fakeExecCommand
			defer func() { utils.System.ExecCommand = exec.Command }()

			fakeUpgradeConvertMasterRequest := &pb.UpgradeConvertMasterRequest{
				OldBinDir: "/old/path/bin",
				NewBinDir: "/new/path/bin"}

			_, err = hub.UpgradeConvertMaster(nil, fakeUpgradeConvertMasterRequest)

			Eventually(testStdout).Should(gbytes.Say("Starting master upgrade"))
			Eventually(testStdout).Should(Not(gbytes.Say("Found no errors when starting the upgrade")))

			Eventually(testStdErr).Should(gbytes.Say("An error occured:"))
			Expect(err).To(BeNil())
		})
	})
})