// Code generated by solo-kit. DO NOT EDIT.

package v1

import (
	"sync"
	"time"

	gloo_solo_io "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	mTranslatorSnapshotIn     = stats.Int64("translator.ingress.solo.io/snap_emitter/snap_in", "The number of snapshots in", "1")
	mTranslatorSnapshotOut    = stats.Int64("translator.ingress.solo.io/snap_emitter/snap_out", "The number of snapshots out", "1")
	mTranslatorSnapshotMissed = stats.Int64("translator.ingress.solo.io/snap_emitter/snap_missed", "The number of snapshots missed", "1")

	translatorsnapshotInView = &view.View{
		Name:        "translator.ingress.solo.io_snap_emitter/snap_in",
		Measure:     mTranslatorSnapshotIn,
		Description: "The number of snapshots updates coming in",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{},
	}
	translatorsnapshotOutView = &view.View{
		Name:        "translator.ingress.solo.io/snap_emitter/snap_out",
		Measure:     mTranslatorSnapshotOut,
		Description: "The number of snapshots updates going out",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{},
	}
	translatorsnapshotMissedView = &view.View{
		Name:        "translator.ingress.solo.io/snap_emitter/snap_missed",
		Measure:     mTranslatorSnapshotMissed,
		Description: "The number of snapshots updates going missed. this can happen in heavy load. missed snapshot will be re-tried after a second.",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{},
	}
)

func init() {
	view.Register(translatorsnapshotInView, translatorsnapshotOutView, translatorsnapshotMissedView)
}

type TranslatorEmitter interface {
	Register() error
	Secret() gloo_solo_io.SecretClient
	Upstream() gloo_solo_io.UpstreamClient
	Ingress() IngressClient
	Snapshots(watchNamespaces []string, opts clients.WatchOpts) (<-chan *TranslatorSnapshot, <-chan error, error)
}

func NewTranslatorEmitter(secretClient gloo_solo_io.SecretClient, upstreamClient gloo_solo_io.UpstreamClient, ingressClient IngressClient) TranslatorEmitter {
	return NewTranslatorEmitterWithEmit(secretClient, upstreamClient, ingressClient, make(chan struct{}))
}

func NewTranslatorEmitterWithEmit(secretClient gloo_solo_io.SecretClient, upstreamClient gloo_solo_io.UpstreamClient, ingressClient IngressClient, emit <-chan struct{}) TranslatorEmitter {
	return &translatorEmitter{
		secret:    secretClient,
		upstream:  upstreamClient,
		ingress:   ingressClient,
		forceEmit: emit,
	}
}

type translatorEmitter struct {
	forceEmit <-chan struct{}
	secret    gloo_solo_io.SecretClient
	upstream  gloo_solo_io.UpstreamClient
	ingress   IngressClient
}

func (c *translatorEmitter) Register() error {
	if err := c.secret.Register(); err != nil {
		return err
	}
	if err := c.upstream.Register(); err != nil {
		return err
	}
	if err := c.ingress.Register(); err != nil {
		return err
	}
	return nil
}

func (c *translatorEmitter) Secret() gloo_solo_io.SecretClient {
	return c.secret
}

func (c *translatorEmitter) Upstream() gloo_solo_io.UpstreamClient {
	return c.upstream
}

func (c *translatorEmitter) Ingress() IngressClient {
	return c.ingress
}

func (c *translatorEmitter) Snapshots(watchNamespaces []string, opts clients.WatchOpts) (<-chan *TranslatorSnapshot, <-chan error, error) {

	if len(watchNamespaces) == 0 {
		watchNamespaces = []string{""}
	}

	for _, ns := range watchNamespaces {
		if ns == "" && len(watchNamespaces) > 1 {
			return nil, nil, errors.Errorf("the \"\" namespace is used to watch all namespaces. Snapshots can either be tracked for " +
				"specific namespaces or \"\" AllNamespaces, but not both.")
		}
	}

	errs := make(chan error)
	var done sync.WaitGroup
	ctx := opts.Ctx
	/* Create channel for Secret */
	type secretListWithNamespace struct {
		list      gloo_solo_io.SecretList
		namespace string
	}
	secretChan := make(chan secretListWithNamespace)

	var initialSecretList gloo_solo_io.SecretList
	/* Create channel for Upstream */
	type upstreamListWithNamespace struct {
		list      gloo_solo_io.UpstreamList
		namespace string
	}
	upstreamChan := make(chan upstreamListWithNamespace)

	var initialUpstreamList gloo_solo_io.UpstreamList
	/* Create channel for Ingress */
	type ingressListWithNamespace struct {
		list      IngressList
		namespace string
	}
	ingressChan := make(chan ingressListWithNamespace)

	var initialIngressList IngressList

	currentSnapshot := TranslatorSnapshot{}

	for _, namespace := range watchNamespaces {
		/* Setup namespaced watch for Secret */
		{
			secrets, err := c.secret.List(namespace, clients.ListOpts{Ctx: opts.Ctx, Selector: opts.Selector})
			if err != nil {
				return nil, nil, errors.Wrapf(err, "initial Secret list")
			}
			initialSecretList = append(initialSecretList, secrets...)
		}
		secretNamespacesChan, secretErrs, err := c.secret.Watch(namespace, opts)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "starting Secret watch")
		}

		done.Add(1)
		go func(namespace string) {
			defer done.Done()
			errutils.AggregateErrs(ctx, errs, secretErrs, namespace+"-secrets")
		}(namespace)
		/* Setup namespaced watch for Upstream */
		{
			upstreams, err := c.upstream.List(namespace, clients.ListOpts{Ctx: opts.Ctx, Selector: opts.Selector})
			if err != nil {
				return nil, nil, errors.Wrapf(err, "initial Upstream list")
			}
			initialUpstreamList = append(initialUpstreamList, upstreams...)
		}
		upstreamNamespacesChan, upstreamErrs, err := c.upstream.Watch(namespace, opts)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "starting Upstream watch")
		}

		done.Add(1)
		go func(namespace string) {
			defer done.Done()
			errutils.AggregateErrs(ctx, errs, upstreamErrs, namespace+"-upstreams")
		}(namespace)
		/* Setup namespaced watch for Ingress */
		{
			ingresses, err := c.ingress.List(namespace, clients.ListOpts{Ctx: opts.Ctx, Selector: opts.Selector})
			if err != nil {
				return nil, nil, errors.Wrapf(err, "initial Ingress list")
			}
			initialIngressList = append(initialIngressList, ingresses...)
		}
		ingressNamespacesChan, ingressErrs, err := c.ingress.Watch(namespace, opts)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "starting Ingress watch")
		}

		done.Add(1)
		go func(namespace string) {
			defer done.Done()
			errutils.AggregateErrs(ctx, errs, ingressErrs, namespace+"-ingresses")
		}(namespace)

		/* Watch for changes and update snapshot */
		go func(namespace string) {
			for {
				select {
				case <-ctx.Done():
					return
				case secretList := <-secretNamespacesChan:
					select {
					case <-ctx.Done():
						return
					case secretChan <- secretListWithNamespace{list: secretList, namespace: namespace}:
					}
				case upstreamList := <-upstreamNamespacesChan:
					select {
					case <-ctx.Done():
						return
					case upstreamChan <- upstreamListWithNamespace{list: upstreamList, namespace: namespace}:
					}
				case ingressList := <-ingressNamespacesChan:
					select {
					case <-ctx.Done():
						return
					case ingressChan <- ingressListWithNamespace{list: ingressList, namespace: namespace}:
					}
				}
			}
		}(namespace)
	}
	/* Initialize snapshot for Secrets */
	currentSnapshot.Secrets = initialSecretList.Sort()
	/* Initialize snapshot for Upstreams */
	currentSnapshot.Upstreams = initialUpstreamList.Sort()
	/* Initialize snapshot for Ingresses */
	currentSnapshot.Ingresses = initialIngressList.Sort()

	snapshots := make(chan *TranslatorSnapshot)
	go func() {
		// sent initial snapshot to kick off the watch
		initialSnapshot := currentSnapshot.Clone()
		snapshots <- &initialSnapshot

		timer := time.NewTicker(time.Second * 1)
		previousHash := currentSnapshot.Hash()
		sync := func() {
			currentHash := currentSnapshot.Hash()
			if previousHash == currentHash {
				return
			}

			sentSnapshot := currentSnapshot.Clone()
			select {
			case snapshots <- &sentSnapshot:
				stats.Record(ctx, mTranslatorSnapshotOut.M(1))
				previousHash = currentHash
			default:
				stats.Record(ctx, mTranslatorSnapshotMissed.M(1))
			}
		}
		secretsByNamespace := make(map[string]gloo_solo_io.SecretList)
		upstreamsByNamespace := make(map[string]gloo_solo_io.UpstreamList)
		ingressesByNamespace := make(map[string]IngressList)

		for {
			record := func() { stats.Record(ctx, mTranslatorSnapshotIn.M(1)) }

			select {
			case <-timer.C:
				sync()
			case <-ctx.Done():
				close(snapshots)
				done.Wait()
				close(errs)
				return
			case <-c.forceEmit:
				sentSnapshot := currentSnapshot.Clone()
				snapshots <- &sentSnapshot
			case secretNamespacedList := <-secretChan:
				record()

				namespace := secretNamespacedList.namespace

				// merge lists by namespace
				secretsByNamespace[namespace] = secretNamespacedList.list
				var secretList gloo_solo_io.SecretList
				for _, secrets := range secretsByNamespace {
					secretList = append(secretList, secrets...)
				}
				currentSnapshot.Secrets = secretList.Sort()
			case upstreamNamespacedList := <-upstreamChan:
				record()

				namespace := upstreamNamespacedList.namespace

				// merge lists by namespace
				upstreamsByNamespace[namespace] = upstreamNamespacedList.list
				var upstreamList gloo_solo_io.UpstreamList
				for _, upstreams := range upstreamsByNamespace {
					upstreamList = append(upstreamList, upstreams...)
				}
				currentSnapshot.Upstreams = upstreamList.Sort()
			case ingressNamespacedList := <-ingressChan:
				record()

				namespace := ingressNamespacedList.namespace

				// merge lists by namespace
				ingressesByNamespace[namespace] = ingressNamespacedList.list
				var ingressList IngressList
				for _, ingresses := range ingressesByNamespace {
					ingressList = append(ingressList, ingresses...)
				}
				currentSnapshot.Ingresses = ingressList.Sort()
			}
		}
	}()
	return snapshots, errs, nil
}
