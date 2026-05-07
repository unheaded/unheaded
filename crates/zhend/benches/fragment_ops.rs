use criterion::{black_box, criterion_group, criterion_main, Criterion};

fn bench_fragment_creation(c: &mut Criterion) {
    use zhend::pu::fragment::Fragment;

    c.bench_function("fragment_new_1kb", |b| {
        let payload = vec![0xAB_u8; 1024];
        b.iter(|| Fragment::new(black_box(payload.clone()), vec![], None));
    });

    c.bench_function("fragment_verify_1kb", |b| {
        let payload = vec![0xAB_u8; 1024];
        let frag = Fragment::new(payload, vec![], None);
        b.iter(|| black_box(frag.verify()));
    });
}

fn bench_cosine_similarity(c: &mut Criterion) {
    use zhend::de::similarity::cosine_similarity;

    c.bench_function("cosine_384d", |b| {
        let a: Vec<u8> = (0..384).map(|i| (i % 256) as u8).collect();
        let b_vec: Vec<u8> = (0..384).map(|i| ((i + 50) % 256) as u8).collect();
        b.iter(|| cosine_similarity(black_box(&a), black_box(&b_vec)));
    });
}

fn bench_codec_roundtrip(c: &mut Criterion) {
    use zhend::pu::codec;
    use zhend::pu::fragment::Fragment;

    c.bench_function("codec_roundtrip_1kb", |b| {
        let frag = Fragment::new(vec![0xCD_u8; 1024], vec![1, 2, 3, 4], None);
        b.iter(|| {
            let encoded = codec::encode(black_box(&frag)).unwrap();
            let _decoded = codec::decode(black_box(&encoded)).unwrap();
        });
    });
}

fn bench_sustained_ingest(c: &mut Criterion) {
    use tempfile::TempDir;
    use zhend::pu::fragment::Fragment;
    use zhend::pu::store::TieredStore;

    let mut group = c.benchmark_group("sustained_ingest");
    group.sample_size(10); // fewer samples since each iteration is 1000 fragments
    group.throughput(criterion::Throughput::Elements(1000));

    group.bench_function("ingest_1000_fragments_256b", |b| {
        b.iter_with_setup(
            || {
                // Setup: create store + pre-build fragments.
                let tmp = TempDir::new().unwrap();
                let store = TieredStore::open(tmp.path()).unwrap();
                let fragments: Vec<Fragment> = (0..1000)
                    .map(|i| {
                        let payload: Vec<u8> = format!("bench_payload_{:08}", i).into_bytes();
                        // Pad to 256 bytes.
                        let mut padded = payload;
                        padded.resize(256, 0xAB);
                        Fragment::new(padded, vec![], None)
                    })
                    .collect();
                (store, fragments, tmp)
            },
            |(store, fragments, _tmp)| {
                for frag in fragments {
                    black_box(store.ingest(frag).unwrap());
                }
            },
        );
    });

    group.bench_function("ingest_1000_fragments_1kb", |b| {
        b.iter_with_setup(
            || {
                let tmp = TempDir::new().unwrap();
                let store = TieredStore::open(tmp.path()).unwrap();
                let fragments: Vec<Fragment> = (0..1000)
                    .map(|i| {
                        let mut payload = format!("bench_payload_{:08}", i).into_bytes();
                        payload.resize(1024, 0xCD);
                        Fragment::new(payload, vec![], None)
                    })
                    .collect();
                (store, fragments, tmp)
            },
            |(store, fragments, _tmp)| {
                for frag in fragments {
                    black_box(store.ingest(frag).unwrap());
                }
            },
        );
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_fragment_creation,
    bench_cosine_similarity,
    bench_codec_roundtrip,
    bench_sustained_ingest,
);
criterion_main!(benches);
