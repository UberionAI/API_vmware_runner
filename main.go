package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/vim25/types"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s is required", key)
	}
	return v
}

func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func httpClient(insecure bool) *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}
	return &http.Client{Transport: tr, Timeout: 3 * time.Minute}
}

func main() {
	_ = godotenv.Load()

	// vCenter settings
	vcHost := mustEnv("VCENTER_HOST")
	vcUser := mustEnv("VCENTER_USER")
	vcPass := mustEnv("VCENTER_PASS")
	insecure := strings.ToLower(os.Getenv("VCENTER_INSECURE")) == "true"
	dc := os.Getenv("VCENTER_DATACENTER")

	// VM settings
	vmName := mustEnv("VM_NAME")
	guestUser := mustEnv("GUEST_USER")
	guestPass := mustEnv("GUEST_PASS")

	// ctx
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// connect to vCenter
	u := &url.URL{
		Scheme: "https",
		Host:   vcHost,
		Path:   "/sdk",
	}
	u.User = url.UserPassword(vcUser, vcPass)

	client, err := govmomi.NewClient(ctx, u, insecure)
	if err != nil {
		log.Fatalf("vCenter connect error: %v", err)
	}
	defer client.Logout(ctx)

	// find VM
	finder := find.NewFinder(client.Client, true)
	if dc != "" {
		dcObj, err := finder.Datacenter(ctx, dc)
		if err != nil {
			log.Fatalf("finder.Datacenter: %v", err)
		}
		finder.SetDatacenter(dcObj)
	}
	vm, err := finder.VirtualMachine(ctx, vmName)
	if err != nil {
		log.Fatalf("VM not found: %v", err)
	}

	ops := guest.NewOperationsManager(client.Client, vm.Reference())

	// auth manager
	authMgr, err := ops.AuthManager(ctx)
	if err != nil {
		log.Fatalf("AuthManager error: %v", err)
	}
	auth := &types.NamePasswordAuthentication{
		Username: guestUser,
		Password: guestPass,
	}
	if err := authMgr.ValidateCredentials(ctx, auth); err != nil {
		log.Fatalf("❌ Ошибка аутентификации: %v", err)
	}
	log.Printf("✅ Аутентификация успешна: %s@%s", guestUser, vmName)

	// managers
	pm, err := ops.ProcessManager(ctx)
	if err != nil {
		log.Fatalf("ProcessManager: %v", err)
	}
	fm, err := ops.FileManager(ctx)
	if err != nil {
		log.Fatalf("FileManager: %v", err)
	}

	// создаём уникальные пути на госте
	suffix := uniqueSuffix()
	scriptPath := fmt.Sprintf("/tmp/govmomi_script_%s.sh", suffix)
	outPath := fmt.Sprintf("/tmp/govmomi_out_%s.out", suffix)

	// читаем script.sh (обязательно должен существовать)
	scriptContent, err := os.ReadFile("script.sh")
	if err != nil {
		log.Fatalf("Read script.sh file: %v", err)
	}

	owner := int32(0)
	group := int32(0)
	fileAttr := &types.GuestPosixFileAttributes{
		OwnerId:     &owner,
		GroupId:     &group,
		Permissions: 0777,
	}

	// upload script
	putURL, err := fm.InitiateFileTransferToGuest(ctx, auth, scriptPath, fileAttr, int64(len(scriptContent)), true)
	if err != nil {
		log.Fatalf("InitiateFileTransferToGuest: %v", err)
	}
	uploadURL, err := fm.TransferURL(ctx, putURL)
	if err != nil {
		log.Fatalf("TransferURL (upload) error: %v", err)
	}

	httpc := httpClient(insecure)
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL.String(), bytes.NewReader(scriptContent))
	if err != nil {
		log.Fatalf("create PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := httpc.Do(req)
	if err != nil {
		log.Fatalf("upload script failed: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Fatalf("upload script bad status: %s", resp.Status)
	}
	log.Printf("✅ Скрипт загружен в guest: %s (size=%d)", scriptPath, len(scriptContent))

	// run script with output redirection
	progPath := "/bin/bash"
	progArgs := fmt.Sprintf(`-lc "sudo /bin/bash '%s' > '%s' 2>&1; echo EXIT:$? >> '%s'"`, scriptPath, outPath, outPath)

	pid, err := pm.StartProgram(ctx, auth, &types.GuestProgramSpec{
		ProgramPath: progPath,
		Arguments:   progArgs,
	})
	if err != nil {
		log.Fatalf("StartProgram: %v", err)
	}
	log.Printf("▶ Запущен скрипт (pid=%d). Жду завершения...", pid)

	// wait until finished
	var procInfo *types.GuestProcessInfo
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		procs, err := pm.ListProcesses(ctx, auth, []int64{pid})
		if err != nil {
			log.Fatalf("ListProcesses: %v", err)
		}
		if len(procs) > 0 {
			procInfo = &procs[0]
			if procInfo.EndTime != nil {
				break
			}
		}
		time.Sleep(1 * time.Second)
	}
	if procInfo != nil {
		log.Printf("✔ Скрипт завершился (exitCode=%d)", procInfo.ExitCode)
	} else {
		log.Printf("⚠ Не получили EndTime от процесса (pid=%d). Попробуем получить вывод всё равно.", pid)
	}

	// download output
	fti, err := fm.InitiateFileTransferFromGuest(ctx, auth, outPath)
	if err != nil {
		log.Fatalf("InitiateFileTransferFromGuest: %v", err)
	}
	downloadURL, err := fm.TransferURL(ctx, fti.Url)
	if err != nil {
		log.Fatalf("TransferURL (download) error: %v", err)
	}

	resp2, err := httpc.Get(downloadURL.String())
	if err != nil {
		log.Fatalf("download output failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp2.Body, 2048))
		log.Fatalf("download output bad status=%s body=%s", resp2.Status, string(body))
	}

	outputFile := fmt.Sprintf("%s.txt", vmName)
	outF, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("cannot create output file: %v", err)
	}
	defer outF.Close()

	_, err = io.Copy(outF, resp2.Body)
	if err != nil {
		log.Fatalf("cannot write output to file: %v", err)
	}

	log.Printf("✅ Вывод сохранён в файл: %s", outputFile)

	// cleanup
	_ = fm.DeleteFile(ctx, auth, scriptPath)
	_ = fm.DeleteFile(ctx, auth, outPath)
	log.Printf("Удалены временные файлы на госте: %s , %s", scriptPath, outPath)
}
