/**
 * VibeChat P2P - Identity & Crypto Module
 * Uses standard Web Crypto API (available in all modern browsers).
 */

const VibeCrypto = {
    async generateKeypair() {
        return await window.crypto.subtle.generateKey(
            { name: "ECDSA", namedCurve: "P-256" },
            true, ["sign", "verify"]
        );
    },

    async exportPublicKey(keyPair) {
        return await window.crypto.subtle.exportKey("jwk", keyPair.publicKey);
    },

    async exportPrivateKey(keyPair) {
        return await window.crypto.subtle.exportKey("jwk", keyPair.privateKey);
    },

    async deriveKeyFromPassword(password, salt) {
        const encoder = new TextEncoder();
        const keyMaterial = await window.crypto.subtle.importKey(
            "raw", encoder.encode(password), { name: "PBKDF2" }, false, ["deriveBits", "deriveKey"]
        );
        return window.crypto.subtle.deriveKey(
            { name: "PBKDF2", salt: salt, iterations: 100000, hash: "SHA-256" },
            keyMaterial, { name: "AES-GCM", length: 256 }, true, ["encrypt", "decrypt"]
        );
    },

    async encryptPrivateKey(jwkObject, password) {
        const salt = window.crypto.getRandomValues(new Uint8Array(16));
        const iv = window.crypto.getRandomValues(new Uint8Array(12));
        const aesKey = await this.deriveKeyFromPassword(password, salt);
        const encoder = new TextEncoder();
        const data = encoder.encode(JSON.stringify(jwkObject));
        const encryptedContent = await window.crypto.subtle.encrypt(
            { name: "AES-GCM", iv: iv }, aesKey, data
        );
        return {
            salt: Array.from(salt), iv: Array.from(iv),
            ciphertext: Array.from(new Uint8Array(encryptedContent))
        };
    },

    async decryptPrivateKey(encryptedData, password) {
        try {
            const salt = new Uint8Array(encryptedData.salt);
            const iv = new Uint8Array(encryptedData.iv);
            const ciphertext = new Uint8Array(encryptedData.ciphertext);
            const aesKey = await this.deriveKeyFromPassword(password, salt);
            const decryptedContent = await window.crypto.subtle.decrypt(
                { name: "AES-GCM", iv: iv }, aesKey, ciphertext
            );
            const decoder = new TextDecoder();
            return JSON.parse(decoder.decode(decryptedContent));
        } catch (e) {
            throw new Error("Invalid password or corrupted data");
        }
    }
};
window.VibeCrypto = VibeCrypto;
