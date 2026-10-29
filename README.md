# tailforward

Use Tailscale without needing admin privileges or installing anything.

tailforward "forwards" requests sent to a local port range to a Tailscale device, so you can access your tailnet devices on a machine where you can't install the Tailscale client.

## Configuring

First, you need to connect to your tailnet. You can choose to untick ephemeral if you want to keep your login persisted. Note that this adds a device to your tailnet which can become cluttered very fast.

<img width="548" height="229" alt="firefox_1b7AEgEoNB" src="https://github.com/user-attachments/assets/33278723-8d51-492b-888b-d20cc7fb9867" />

Now you can add forwarding rules:

`device`: The tailnet device name. It autocompletes detected devices, but you can use any name.<br>
`from`: The start of the port range **on the tailnet device**<br>
`to`: The end of the port range **on the tailnet device**<br>
`local`: The start of the port range to listen on locally (end gets inferred)

<img width="552" height="231" alt="image" src="https://github.com/user-attachments/assets/ce2ef5dd-9d3f-4bba-9943-bfd758d407c2" />

To log out of tailscale, you can press "reauth ts"

## Encrypting

You can optionally encrypt your state on disk. To do this, simply click on "encrypt" and enter a key:

<img width="557" height="144" alt="firefox_ATERolreob" src="https://github.com/user-attachments/assets/b432012c-dfee-46fc-bce7-485311eed9b8" />

You can then manage your encryption:

`mod key`: Change the key to something different<br>
`rm key`: Remove encryption key -> files become unencrypted aswell

## License

tailforward is licensed under the MIT License.

<hr>

tailforward is not affiliated or endorsed by Tailscale, Inc.