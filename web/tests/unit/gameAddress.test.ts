import { expect, test } from "vitest";
import { NetworkProtocol } from "../../src/contracts/domains";
import {
  primaryGamePort,
  resolveGameAddress,
} from "../../src/utils/gameAddress";

const game = {
  bind_ip: "192.0.2.1",
  container_port: 25565,
  host_port: 25570,
  protocol: NetworkProtocol.TCP,
};
const query = { ...game, container_port: 25575, host_port: 25580 };
const required = [
  { container: 25565, protocol: NetworkProtocol.TCP, primary: true },
];

test("connection overrides remain literal and defaults need a primary host port", () => {
  for (const address of [
    "https://Play.Example.test:32100/Join?World=Case",
    "play.example.test",
    "connect 192.0.2.1:27015",
  ])
    expect(resolveGameAddress(address, "ignored.example", 25565)).toBe(address);
  expect(resolveGameAddress("", "games.example.test", 25565)).toBe(
    "games.example.test:25565",
  );
  expect(resolveGameAddress("", "2001:db8::1", 25565)).toBe(
    "[2001:db8::1]:25565",
  );
  expect(resolveGameAddress("", "192.0.2.1", 25565)).toBe("192.0.2.1:25565");
  expect(resolveGameAddress("", "", 25565)).toBe("");
  expect(resolveGameAddress("", "games.example.test", 0)).toBe("");
  expect(resolveGameAddress("", "games.example.test", 65536)).toBe("");
});

test("unusable canonical Spark defaults stay hidden while overrides remain literal", () => {
  for (const host of [
    "0.0.0.0",
    "::",
    "224.0.0.1",
    "239.255.255.255",
    "ff02::1",
    "0:0:0:0:0:0:0:0",
    "::ffff:224.0.0.1",
    "::ffff:0.0.0.0",
  ]) {
    expect(resolveGameAddress("", host, 25565)).toBe("");
    expect(resolveGameAddress("custom connection", host, 25565)).toBe(
      "custom connection",
    );
  }
  for (const host of [
    "192.168.1.10",
    "127.0.0.1",
    "games.example.test",
    "2001:db8::1",
  ]) {
    expect(resolveGameAddress("", host, 25565)).not.toBe("");
  }
});

test("only an explicit primary designation selects a mapping", () => {
  expect(primaryGamePort([query, game], required)).toBe(25570);
  expect(primaryGamePort([query, game], undefined)).toBe(0);
  expect(primaryGamePort([query], required)).toBe(0);
  expect(primaryGamePort([game], undefined)).toBe(0);
  expect(
    primaryGamePort(
      [game, { ...game, protocol: NetworkProtocol.UDP }],
      required,
    ),
  ).toBe(25570);
  expect(resolveGameAddress("", "", primaryGamePort([game], required))).toBe(
    "",
  );
});
