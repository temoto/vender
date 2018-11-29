# -*- mode: ruby -*-
# vi: set ft=ruby :

Vagrant.configure("2") do |config|
  config.vm.box = "generic/debian9"
  config.vm.box_check_update = false
  # config.vm.network "forwarded_port", guest: 80, host: 8080, host_ip: "127.0.0.1"
  config.vm.synced_folder ".", "/vender"

  config.vm.provider "virtualbox" do |vb|
    #vb.gui = true
    vb.memory = "1524"
  end
  #
  # View the documentation for the provider you are using for more
  # information on available options.

  config.vm.provision "shell", inline: <<-SHELL
    apt-get update
    apt-get install -y libclang-dev protobuf-compiler
  SHELL
end
