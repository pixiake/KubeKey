/*
Copyright 2020 The KubeSphere Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ssh

import (
	"github.com/pkg/errors"
	"github.com/pkg/sftp"
	"github.com/tmc/scp"
)

func (c *connection) sftp() (*sftp.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sshclient == nil {
		return nil, errors.New("connection closed")
	}

	if c.sftpclient == nil {
		s, err := sftp.NewClient(c.sshclient)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to get sftp.Client")
		}
		c.sftpclient = s
	}

	return c.sftpclient, nil
}

func (c *connection) Scp(src, dst string) error {
	session, err := c.session()

	err = scp.CopyPath(src, dst, session)
	if err != nil {
		return err
	}

	return nil
}
