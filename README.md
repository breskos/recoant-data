# recoant-data

Data for the tests, examples and benchmarks of [recoant](https://github.com/breskos/recoant). It lives in its own repository so that the recoant module stays small.

## Synthetic data

The [synthetic](synthetic) folder holds generated data sets with a known structure, so you can tell how well a recommender should do. They contain no third-party and no personal data. Every data set is a folder with a `signals.csv` (time, subject, object, signal type, value, session) and tables with the hidden structure that generated it.

| Folder | Use it for | Data sets |
| --- | --- | --- |
| [basic](synthetic/basic) | Batch recommendation | pure Zipf popularity, taste clusters |
| [latent](synthetic/latent) | SVD/PCA, matrix factorization, ALS, BPR | Netflix-like star ratings, implicit feedback from latent factors |
| [funnel](synthetic/funnel) | Funnel modeling, signal relevance | e-commerce sessions with product quality, informative vs. noise signal types |
| [stream](synthetic/stream) | Online and incremental learning | trending news, preference drift |
| [session](synthetic/session) | Session-based, next-item | anonymous sessions along item chains |
| [relations](synthetic/relations) | User-to-user, graph models | follow graph, product–brand–category relations |
| [loading](synthetic/loading) | Loaders and ingestion | edge cases, invalid rows |
| [benchmark](synthetic/benchmark) | Performance | 3.5 million signals, 200,000 users (gzip) |

[synthetic/README.md](synthetic/README.md) describes every set, its files and what a model should achieve on it.

The folder is a small Go module (standard library only, no dependency on recoant) with a catalog, a reader and tests that check every file and its hidden structure:

```
cd synthetic && go test ./...
```

The files are reproducible; CI regenerates them and fails on any difference. The generators are in the module:

```
cd synthetic && go run ./cmd/gendata -out .
```

## License

Apache License 2.0, see [LICENSE](LICENSE).
