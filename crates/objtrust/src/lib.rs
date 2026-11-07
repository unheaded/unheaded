// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//! Whether an eBPF object on disk may be loaded: the Rust twin of
//! `pkg/ebpf/objtrust`. Every Rust loader calls [`check`] before it reads an
//! object, whatever user it runs as: a process with CAP_BPF and no root can put
//! a program in the kernel too.
//!
//! What it guarantees: only root could have written the file, replaced any
//! directory above it or repointed any symlink on the way to it, at the moment
//! of the check. So the path a caller opens afterwards still leads there.
//!
//! What it cannot cover: integrity (it reads owners and modes, not bytes), root
//! itself, and pinned objects, which have no file to check. A build tree is not
//! a trust anchor, so a loader pointed at one needs [`ALLOW_FLAG`], which is for
//! development only.

use std::collections::VecDeque;
use std::ffi::OsString;
use std::fmt;
use std::io;
use std::os::unix::fs::MetadataExt;
use std::path::{Component, Path, PathBuf};

/// The opt-out every Rust loader takes, spelled as trace-collector spells it.
pub const ALLOW_FLAG: &str = "--allow-untrusted-ebpf-obj-dir";

/// How many symlinks one path may pass through, as in the kernel.
const MAX_LINKS: usize = 40;

const S_IFMT: u32 = 0o170000;
const S_IFLNK: u32 = 0o120000;
const GROUP_OR_WORLD_WRITE: u32 = 0o022;

/// An object that a non-root user could have written.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Untrusted(String);

impl fmt::Display for Untrusted {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "eBPF object path is writable by a non-root user: {} (install it root-owned, or pass {ALLOW_FLAG} for a build tree)",
            self.0
        )
    }
}

impl std::error::Error for Untrusted {}

/// Owner and `st_mode` of one path, not following a final symlink.
#[derive(Debug, Clone, Copy)]
struct Entry {
    uid: u32,
    mode: u32,
}

impl Entry {
    fn is_symlink(self) -> bool {
        self.mode & S_IFMT == S_IFLNK
    }
}

/// Applies the rule to `path` with the real owners and modes. A relative path
/// is taken from the working directory, as open(2) takes it.
pub fn check(path: &Path) -> Result<(), Untrusted> {
    let absolute = if path.is_absolute() {
        path.to_path_buf()
    } else {
        let cwd = std::env::current_dir()
            .map_err(|e| Untrusted(format!("resolve {}: {e}", path.display())))?;
        cwd.join(path)
    };
    chain_error(&absolute, &lstat_owner, &|p| std::fs::read_link(p))
}

/// [`check`], or nothing when the operator passed [`ALLOW_FLAG`].
pub fn check_unless_allowed(path: &Path, allow_untrusted: bool) -> Result<(), Untrusted> {
    if allow_untrusted {
        return Ok(());
    }
    check(path)
}

fn lstat_owner(p: &Path) -> io::Result<Entry> {
    let m = std::fs::symlink_metadata(p)?;
    Ok(Entry {
        uid: m.uid(),
        mode: m.mode(),
    })
}

/// ssh StrictModes semantics for an object about to be loaded: the file, every
/// directory above it and every symlink met on the way must be owned by root,
/// and the file and directories must not be group- or world-writable. It walks
/// `path` one component at a time, as the kernel does, following links as it
/// meets them, so a link's own directory is checked and not only where the link
/// ends up. `path` must be absolute.
fn chain_error(
    path: &Path,
    stat: &dyn Fn(&Path) -> io::Result<Entry>,
    readlink: &dyn Fn(&Path) -> io::Result<PathBuf>,
) -> Result<(), Untrusted> {
    trusted_entry(Path::new("/"), stat)?;
    let mut cur = PathBuf::from("/");
    let mut links = 0;
    let mut pending: VecDeque<OsString> = names(path).collect();
    while let Some(name) = pending.pop_front() {
        if name == ".." {
            cur.pop();
            continue;
        }
        let next = cur.join(&name);
        if !trusted_entry(&next, stat)?.is_symlink() {
            cur = next;
            continue;
        }
        links += 1;
        if links > MAX_LINKS {
            return Err(Untrusted(format!(
                "{} passes through more than {MAX_LINKS} symlinks",
                path.display()
            )));
        }
        let target =
            readlink(&next).map_err(|e| Untrusted(format!("readlink {}: {e}", next.display())))?;
        if target.is_absolute() {
            cur = PathBuf::from("/");
        }
        for part in names(&target).collect::<Vec<_>>().into_iter().rev() {
            pending.push_front(part);
        }
    }
    Ok(())
}

/// The names to walk in `p`: `.` and the root are no step at all.
fn names(p: &Path) -> impl Iterator<Item = OsString> + '_ {
    p.components().filter_map(|c| match c {
        Component::Normal(n) => Some(n.to_os_string()),
        Component::ParentDir => Some(OsString::from("..")),
        Component::RootDir | Component::CurDir | Component::Prefix(_) => None,
    })
}

/// One file, directory or symlink. A symlink's mode means nothing: only its
/// owner and its directory decide who can repoint it.
fn trusted_entry(p: &Path, stat: &dyn Fn(&Path) -> io::Result<Entry>) -> Result<Entry, Untrusted> {
    let e = stat(p).map_err(|err| Untrusted(format!("stat {}: {err}", p.display())))?;
    if e.uid != 0 {
        return Err(Untrusted(format!(
            "{} is owned by uid {}, not root",
            p.display(),
            e.uid
        )));
    }
    if !e.is_symlink() && e.mode & GROUP_OR_WORLD_WRITE != 0 {
        return Err(Untrusted(format!(
            "{} is group- or world-writable ({:o})",
            p.display(),
            e.mode & 0o7777
        )));
    }
    Ok(e)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    const DIR: u32 = 0o040000;
    const FILE: u32 = 0o100000;

    type Tree = HashMap<&'static str, (u32, u32)>;

    fn good() -> Tree {
        HashMap::from([
            ("/", (0, DIR | 0o755)),
            ("/usr", (0, DIR | 0o755)),
            ("/usr/lib", (0, DIR | 0o755)),
            ("/usr/lib/unheaded", (0, DIR | 0o755)),
            ("/usr/lib/unheaded/prog.o", (0, FILE | 0o644)),
        ])
    }

    fn walk(
        tree: &Tree,
        links: &HashMap<&'static str, &'static str>,
        path: &str,
    ) -> Result<(), Untrusted> {
        let stat = |p: &Path| -> io::Result<Entry> {
            tree.get(p.to_str().unwrap())
                .map(|&(uid, mode)| Entry { uid, mode })
                .ok_or_else(|| io::Error::from(io::ErrorKind::NotFound))
        };
        let readlink = |p: &Path| -> io::Result<PathBuf> {
            links
                .get(p.to_str().unwrap())
                .map(PathBuf::from)
                .ok_or_else(|| io::Error::from(io::ErrorKind::InvalidInput))
        };
        chain_error(Path::new(path), &stat, &readlink)
    }

    #[test]
    fn a_root_owned_chain_is_accepted() {
        walk(&good(), &HashMap::new(), "/usr/lib/unheaded/prog.o").unwrap();
    }

    #[test]
    fn every_way_a_user_could_write_it_is_refused() {
        let cases: [(&str, &str, (u32, u32)); 6] = [
            (
                "file owned by a user",
                "/usr/lib/unheaded/prog.o",
                (1000, FILE | 0o644),
            ),
            (
                "file group-writable",
                "/usr/lib/unheaded/prog.o",
                (0, FILE | 0o664),
            ),
            (
                "parent owned by a user",
                "/usr/lib/unheaded",
                (1000, DIR | 0o755),
            ),
            ("grandparent world-writable", "/usr/lib", (0, DIR | 0o777)),
            ("sticky world-writable", "/usr/lib", (0, DIR | 0o1777)),
            ("root directory writable", "/", (0, DIR | 0o775)),
        ];
        for (name, path, entry) in cases {
            let mut t = good();
            t.insert(path, entry);
            assert!(
                walk(&t, &HashMap::new(), "/usr/lib/unheaded/prog.o").is_err(),
                "{name} was accepted"
            );
        }
    }

    #[test]
    fn a_stat_that_fails_is_refused() {
        let mut t = good();
        t.remove("/usr");
        assert!(walk(&t, &HashMap::new(), "/usr/lib/unheaded/prog.o").is_err());
    }

    #[test]
    fn a_root_link_in_a_root_directory_is_followed() {
        let mut t = good();
        t.insert("/usr/lib/unheaded/current", (0, S_IFLNK | 0o777));
        let links = HashMap::from([("/usr/lib/unheaded/current", "prog.o")]);
        walk(&t, &links, "/usr/lib/unheaded/current").unwrap();
    }

    #[test]
    fn a_link_is_held_to_its_target_too() {
        let mut t = good();
        t.insert("/usr/lib/unheaded/current", (0, S_IFLNK | 0o777));
        t.insert("/home", (0, DIR | 0o755));
        t.insert("/home/user", (1000, DIR | 0o755));
        t.insert("/home/user/prog.o", (0, FILE | 0o644));
        let links = HashMap::from([("/usr/lib/unheaded/current", "../../../home/user/prog.o")]);
        let err = walk(&t, &links, "/usr/lib/unheaded/current").unwrap_err();
        assert!(err.to_string().contains("/home/user"), "{err}");
    }

    // The same name sits under the link's directory, root's, and at the real
    // target, a user's: walking on from the link would pass the wrong file.
    #[test]
    fn an_absolute_link_target_starts_again_at_the_root() {
        let mut t = good();
        t.insert("/usr/lib/unheaded/current", (0, S_IFLNK | 0o777));
        t.insert("/usr/lib/unheaded/opt", (0, DIR | 0o755));
        t.insert("/usr/lib/unheaded/opt/prog.o", (0, FILE | 0o644));
        t.insert("/opt", (0, DIR | 0o755));
        t.insert("/opt/prog.o", (0, FILE | 0o644));
        let links = HashMap::from([("/usr/lib/unheaded/current", "/opt/prog.o")]);
        walk(&t, &links, "/usr/lib/unheaded/current").unwrap();

        t.insert("/opt/prog.o", (1000, FILE | 0o644));
        let err = walk(&t, &links, "/usr/lib/unheaded/current").unwrap_err();
        assert!(err.to_string().contains("/opt/prog.o"), "{err}");
    }

    #[test]
    fn a_link_a_user_owns_is_refused_wherever_it_points() {
        let mut t = good();
        t.insert("/usr/lib/unheaded/current", (1000, S_IFLNK | 0o777));
        let links = HashMap::from([("/usr/lib/unheaded/current", "prog.o")]);
        assert!(walk(&t, &links, "/usr/lib/unheaded/current").is_err());
    }

    #[test]
    fn a_link_loop_ends_in_a_refusal() {
        let mut t = good();
        t.insert("/usr/lib/unheaded/a", (0, S_IFLNK | 0o777));
        let links = HashMap::from([("/usr/lib/unheaded/a", "a")]);
        let err = walk(&t, &links, "/usr/lib/unheaded/a").unwrap_err();
        assert!(err.to_string().contains("symlinks"), "{err}");
    }

    #[test]
    fn dot_dot_climbs_the_real_path() {
        walk(
            &good(),
            &HashMap::new(),
            "/usr/lib/../lib/./unheaded/prog.o",
        )
        .unwrap();
    }

    // The real stat path: /etc/passwd sits in a root-owned, non-writable chain
    // on any Linux host; a file this test creates never does.
    #[test]
    fn the_real_filesystem() {
        check(Path::new("/etc/passwd")).unwrap();
        let dir = scratch("real");
        let p = dir.join("prog.o");
        std::fs::write(&p, b"x").unwrap();
        let err = check(&p).unwrap_err();
        assert!(err.to_string().contains(ALLOW_FLAG), "{err}");
        check_unless_allowed(&p, true).unwrap();
        assert!(check_unless_allowed(&p, false).is_err());
        std::fs::remove_dir_all(dir).ok();
    }

    // The link's target is root's, but its directory is not: whoever owns that
    // directory could repoint the link between this check and the load.
    #[test]
    fn a_link_in_a_user_directory_is_refused() {
        let dir = scratch("link");
        let p = dir.join("prog.o");
        std::os::unix::fs::symlink("/etc/passwd", &p).unwrap();
        assert!(check(&p).is_err());
        std::fs::remove_dir_all(dir).ok();
    }

    fn scratch(tag: &str) -> PathBuf {
        let d = std::env::temp_dir().join(format!("objtrust-{tag}-{}", std::process::id()));
        std::fs::create_dir_all(&d).unwrap();
        d
    }
}
