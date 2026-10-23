The design and storage format of Duplicacy are documented in this repository:

* [Architecture](docs/ARCHITECTURE.md): code layout and the data flow of each command
* [Storage format](docs/FORMAT.md): config, chunks, snapshots, fossils
* [Prune safety](docs/PRUNE_SAFETY.md): lock-free deletion with two-step fossil collection
* [Security model](docs/SECURITY_MODEL.md): encryption and what an attacker can do
* [Decision records](docs/adr/README.md)

Background on lock-free deduplication is also in the [upstream wiki](https://github.com/gilbertchen/duplicacy/wiki/Lock-Free-Deduplication)
and the [paper](duplicacy_paper.pdf).
