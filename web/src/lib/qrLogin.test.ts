import { generateIdentity, open, seal } from "@/crypto/keys";
import { parseQrLink, qrLink } from "./qrLogin";
import { wrapKeyFromPrf } from "./passkeys";
import { unwrapPrivateKey, wrapPrivateKey } from "@/crypto/keys";

describe("QR sign-in codes", () => {
  it("carry the request and the one-time key, and nothing else reads as one", () => {
    const eph = generateIdentity();
    const link = qrLink("https://nexa.pm", "AbCdEfGhIjKlMnOpQrStUv", eph.publicKey);
    expect(link.startsWith("https://nexa.pm/login/qr#AbCdEfGhIjKlMnOpQrStUv.")).toBe(true);
    const got = parseQrLink(link);
    expect(got?.id).toBe("AbCdEfGhIjKlMnOpQrStUv");
    expect(got?.publicKey).toEqual(eph.publicKey);
    expect(parseQrLink("#short.abc")).toBeNull();
    expect(parseQrLink("https://nexa.pm/login/qr")).toBeNull();
  });

  it("hand over an identity only the waiting device opens", async () => {
    const eph = generateIdentity();
    const me = generateIdentity();
    const box = await seal(eph.publicKey, me.privateKey);
    expect(await open(eph.privateKey, eph.publicKey, box)).toEqual(me.privateKey);
    await expect(open(generateIdentity().privateKey, eph.publicKey, box)).rejects.toThrow();
  });
});

describe("passkey wrapping", () => {
  it("opens with the same PRF output and no other", async () => {
    const me = generateIdentity();
    const prf = new Uint8Array(32).fill(7);
    const wrapped = await wrapPrivateKey(me.privateKey, wrapKeyFromPrf(prf));
    expect(await unwrapPrivateKey(wrapped, wrapKeyFromPrf(prf))).toEqual(me.privateKey);
    await expect(unwrapPrivateKey(wrapped, wrapKeyFromPrf(new Uint8Array(32).fill(8)))).rejects.toThrow();
  });
});
