NF==3 && $1 !~ /^-$/ {
    add[$3] += $1
    del[$3] += $2
    touches[$3]++
}
END {
    for (f in add) {
        total = add[f] + del[f]
        printf "%d\t%d\t%d\t%d\t%s\n", total, add[f], del[f], touches[f], f
    }
}
