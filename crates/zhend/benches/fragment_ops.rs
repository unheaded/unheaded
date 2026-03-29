use criterion::{black_box, criterion_group, criterion_main, Criterion};

fn bench_fragment_creation(c: &mut Criterion) {
    use zhend::pu::fragment::Fragment;

    c.bench_function("fragment_new_1kb", |b| {
        let payload = vec![0xAB_u8; 1024];
        b.iter(|| {
            Fragment::new(black_box(payload.clone()), vec![], None)
        });
    });

    c.bench_function("fragment_verify_1kb", |b| {
        let payload = vec![0xAB_u8; 1024];
        let frag = Fragment::new(payload, vec![], None);
        b.iter(|| {
            black_box(frag.verify())
        });
    });
}

fn bench_cosine_similarity(c: &mut Criterion) {
    use zhend::de::similarity::cosine_similarity;

    c.bench_function("cosine_384d", |b| {
        let a: Vec<u8> = (0..384).map(|i| (i % 256) as u8).collect();
        let b_vec: Vec<u8> = (0..384).map(|i| ((i + 50) % 256) as u8).collect();
        b.iter(|| {
            cosine_similarity(black_box(&a), black_box(&b_vec))
        });
    });
}

fn bench_codec_roundtrip(c: &mut Criterion) {
    use zhend::pu::fragment::Fragment;
    use zhend::pu::codec;

    c.bench_function("codec_roundtrip_1kb", |b| {
        let frag = Fragment::new(vec![0xCD_u8; 1024], vec![1, 2, 3, 4], None);
        b.iter(|| {
            let encoded = codec::encode(black_box(&frag)).unwrap();
            let _decoded = codec::decode(black_box(&encoded)).unwrap();
        });
    });
}

criterion_group!(
    benches,
    bench_fragment_creation,
    bench_cosine_similarity,
    bench_codec_roundtrip,
);
criterion_main!(benches);
