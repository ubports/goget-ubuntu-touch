//
// Helpers to work with an Ubuntu image based Upgrade implementation
//
// Copyright (c) 2013 Canonical Ltd.
//
// Written by Sergio Schvezov <sergio.schvezov@canonical.com>
//
package ubuntuimage

// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License version 3, as published
// by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranties of
// MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
// PURPOSE.  See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along
// with this program.  If not, see <http://www.gnu.org/licenses/>.

import (
	"sort"
)

// By is the type of a "less" function that defines the ordering of its File arguments.
type By func(p1, p2 *File) bool

// Sort is a method on the function type, By, that sorts the argument slice according to the function.
func (by By) Sort(files []File) {
	fs := &fileSorter{
		files: files,
		by:    by,
	}
	sort.Sort(fs)
}

// fileSorter joins a By function and a slice of File to be sorted.
type fileSorter struct {
	files []File
	by    func(p1, p2 *File) bool // Closure used in the Less method.
}

// Len is part of sort.Interface.
func (s *fileSorter) Len() int {
	return len(s.files)
}

// Swap is part of sort.Interface.
func (s *fileSorter) Swap(i, j int) {
	s.files[i], s.files[j] = s.files[j], s.files[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *fileSorter) Less(i, j int) bool {
	return s.by(&s.files[i], &s.files[j])
}

// By is the type of a "less" function that defines the ordering of its File arguments.
type ImageBy func(p1, p2 *Image) bool

// Sort is a method on the function type, By, that sorts the argument slice according to the function.
func (by ImageBy) ImageSort(images []Image) {
	is := &imageSorter{
		images: images,
		by:     by,
	}
	sort.Sort(is)
}

// fileSorter joins a By function and a slice of File to be sorted.
type imageSorter struct {
	images []Image
	by     func(p1, p2 *Image) bool // Closure used in the Less method.
}

// Len is part of sort.Interface.
func (s *imageSorter) Len() int {
	return len(s.images)
}

// Swap is part of sort.Interface.
func (s *imageSorter) Swap(i, j int) {
	s.images[i], s.images[j] = s.images[j], s.images[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *imageSorter) Less(i, j int) bool {
	return s.by(&s.images[i], &s.images[j])
}
