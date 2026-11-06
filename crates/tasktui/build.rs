//! En macOS incrusta `Info.plist` en el binario: EventKit exige las
//! descripciones de uso (`NSReminders…UsageDescription`) para pedir permiso.

fn main() {
    println!("cargo:rerun-if-changed=Info.plist");
    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("macos") {
        let plist = std::path::Path::new(&std::env::var("CARGO_MANIFEST_DIR").unwrap()).join("Info.plist");
        println!("cargo:rustc-link-arg-bins=-Wl,-sectcreate,__TEXT,__info_plist,{}", plist.display());
    }
}
