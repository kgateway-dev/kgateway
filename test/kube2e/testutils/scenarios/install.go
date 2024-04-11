package scenarios

type InstallScenario interface {
	Install()
	Upgrade()

	Uninstall()
}

/**

resourceClientset, err = kube2e.NewDefaultKubeResourceClientSet(ctx)
Expect(err).NotTo(HaveOccurred(), "can create kube resource client set")

clientScheme, err = testutils.BuildClientScheme()
Expect(err).NotTo(HaveOccurred(), "can build client scheme")

kubeClient, err = testutils.GetClient(kubeCtx, clientScheme)
Expect(err).NotTo(HaveOccurred(), "can create client")
*/
